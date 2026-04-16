"""Background process manager — run and monitor long-lived processes."""

import asyncio
import time
import os
import sys
import signal
from collections import deque
from pathlib import Path


def _setup_parent_death_cleanup():
    """Ensure child processes die when acorn exits, even on crash/SIGKILL.
    - Windows: Job Object with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
    - Unix: prctl PR_SET_PDEATHSIG (per-child, set in preexec_fn)
    """
    if sys.platform == 'win32':
        try:
            import ctypes
            from ctypes import wintypes

            kernel32 = ctypes.windll.kernel32

            # Create a Job Object
            job = kernel32.CreateJobObjectW(None, None)
            if not job:
                return None

            # Set it to kill all children when the job handle closes (= process exits)
            class JOBOBJECT_BASIC_LIMIT_INFORMATION(ctypes.Structure):
                _fields_ = [
                    ('PerProcessUserTimeLimit', ctypes.c_int64),
                    ('PerJobUserTimeLimit', ctypes.c_int64),
                    ('LimitFlags', wintypes.DWORD),
                    ('MinimumWorkingSetSize', ctypes.c_size_t),
                    ('MaximumWorkingSetSize', ctypes.c_size_t),
                    ('ActiveProcessLimit', wintypes.DWORD),
                    ('Affinity', ctypes.POINTER(ctypes.c_ulong)),
                    ('PriorityClass', wintypes.DWORD),
                    ('SchedulingClass', wintypes.DWORD),
                ]

            class JOBOBJECT_EXTENDED_LIMIT_INFORMATION(ctypes.Structure):
                _fields_ = [
                    ('BasicLimitInformation', JOBOBJECT_BASIC_LIMIT_INFORMATION),
                    ('IoInfo', ctypes.c_byte * 48),
                    ('ProcessMemoryLimit', ctypes.c_size_t),
                    ('JobMemoryLimit', ctypes.c_size_t),
                    ('PeakProcessMemoryUsed', ctypes.c_size_t),
                    ('PeakJobMemoryUsed', ctypes.c_size_t),
                ]

            JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE = 0x00002000
            info = JOBOBJECT_EXTENDED_LIMIT_INFORMATION()
            info.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE

            kernel32.SetInformationJobObject(
                job, 9,  # JobObjectExtendedLimitInformation
                ctypes.byref(info), ctypes.sizeof(info)
            )
            return job
        except Exception:
            return None
    return None  # Unix uses preexec_fn per-child


def _assign_to_job(job, proc):
    """Assign a subprocess to a Windows Job Object."""
    if not job or sys.platform != 'win32':
        return
    try:
        import ctypes
        handle = ctypes.windll.kernel32.OpenProcess(0x1F0FFF, False, proc.pid)  # PROCESS_ALL_ACCESS
        if handle:
            ctypes.windll.kernel32.AssignProcessToJobObject(job, handle)
            ctypes.windll.kernel32.CloseHandle(handle)
    except Exception:
        pass


def _unix_preexec():
    """On Unix, set PDEATHSIG so child gets SIGTERM when parent dies."""
    try:
        import ctypes
        libc = ctypes.CDLL('libc.so.6', use_errno=True)
        PR_SET_PDEATHSIG = 1
        libc.prctl(PR_SET_PDEATHSIG, signal.SIGTERM)
    except Exception:
        pass


class BackgroundProcess:
    """A single background process with captured output."""

    def __init__(self, pid, command, proc, log_path=None):
        self.id = pid
        self.command = command
        self.proc = proc
        self.output = deque(maxlen=500)  # last 500 lines in memory
        self.log_path = log_path  # persistent file on disk
        self.started = time.time()
        self.ended = None
        self.exit_code = None
        self._task = None
        self._log_file = None

    @property
    def running(self):
        return self.proc.returncode is None

    @property
    def elapsed(self):
        end = self.ended or time.time()
        secs = int(end - self.started)
        if secs < 60:
            return f'{secs}s'
        elif secs < 3600:
            return f'{secs // 60}m {secs % 60}s'
        return f'{secs // 3600}h {(secs % 3600) // 60}m'

    def kill(self):
        try:
            self.proc.kill()
        except Exception:
            pass


class ProcessManager:
    """Manages background processes launched by the agent or user."""

    def __init__(self, log_dir=None):
        self._processes = {}
        self._next_id = 1
        self._log_dir = Path(log_dir) if log_dir else None
        if self._log_dir:
            self._log_dir.mkdir(parents=True, exist_ok=True)
        # Windows: Job Object ensures children die with parent
        self._job = _setup_parent_death_cleanup()

    async def launch(self, command: str, cwd: str) -> BackgroundProcess:
        """Launch a command in the background and start capturing output."""
        kwargs = {}
        if sys.platform != 'win32':
            kwargs['preexec_fn'] = _unix_preexec
        proc = await asyncio.create_subprocess_shell(
            command, cwd=cwd,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.STDOUT,
            **kwargs,
        )
        # Windows: assign to job object so it dies with acorn
        if self._job:
            _assign_to_job(self._job, proc)
        pid = self._next_id
        self._next_id += 1

        log_path = None
        if self._log_dir:
            log_path = str(self._log_dir / f'bg-{pid}.log')

        bp = BackgroundProcess(pid, command, proc, log_path=log_path)

        # Open log file for writing
        if log_path:
            try:
                bp._log_file = open(log_path, 'w', buffering=1)
                bp._log_file.write(f'# Command: {command}\n')
                bp._log_file.write(f'# Started: {time.strftime("%Y-%m-%d %H:%M:%S")}\n')
                bp._log_file.write(f'# CWD: {cwd}\n\n')
            except Exception:
                bp._log_file = None

        self._processes[pid] = bp
        bp._task = asyncio.create_task(self._read_output(bp))
        return bp

    async def _read_output(self, bp: BackgroundProcess):
        """Read stdout lines, store in memory buffer and write to log file."""
        try:
            while True:
                line = await bp.proc.stdout.readline()
                if not line:
                    break
                decoded = line.decode('utf-8', errors='replace').rstrip('\n')
                bp.output.append(decoded)
                if bp._log_file:
                    try:
                        bp._log_file.write(decoded + '\n')
                    except Exception:
                        pass
        except Exception:
            pass
        finally:
            try:
                await bp.proc.wait()
            except Exception:
                pass
            bp.exit_code = bp.proc.returncode
            bp.ended = time.time()
            if bp._log_file:
                try:
                    bp._log_file.write(f'\n# Exited: code={bp.exit_code} at {time.strftime("%Y-%m-%d %H:%M:%S")}\n')
                    bp._log_file.close()
                except Exception:
                    pass
                bp._log_file = None

    def list_all(self):
        """Return all processes (running + finished)."""
        return list(self._processes.values())

    def get(self, pid: int):
        return self._processes.get(pid)

    def kill(self, pid: int) -> bool:
        bp = self._processes.get(pid)
        if bp and bp.running:
            bp.kill()
            return True
        return False

    def remove(self, pid: int):
        bp = self._processes.get(pid)
        if bp and not bp.running:
            del self._processes[pid]
            return True
        return False

    @property
    def running_count(self):
        return sum(1 for bp in self._processes.values() if bp.running)

    def kill_all(self):
        """Kill all running background processes. Called on exit."""
        for bp in self._processes.values():
            if bp.running:
                try:
                    bp.kill()
                except Exception:
                    pass
