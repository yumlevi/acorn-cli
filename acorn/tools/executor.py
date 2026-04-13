"""Tool dispatch — routes tool:request to local handlers or signals server fallback."""

from acorn.tools import file_ops, shell, search

# Tools executed locally on the user's machine
LOCAL_TOOLS = {'exec', 'read_file', 'write_file', 'edit_file', 'glob', 'grep', 'web_fetch'}

# Tools that must stay server-side (operate on Anima internals)
SERVER_TOOLS = {
    'graph_query', 'graph_update', 'graph_delete', 'query_about',
    'message_send', 'message_react', 'message_edit', 'message_read',
    'delegate_task', 'task_status', 'task_cancel', 'task_update',
    'save_tool', 'skill_lookup', 'skill_update',
    'session_status', 'sessions_list', 'env_manage',
    'web_serve', 'notify_user', 'web_search',
    'anima_list', 'anima_message', 'anima_graph', 'anima_manage',
    'browser', 'startup_tasks', 'data_poller',
    'remote_exec', 'remote_read_file', 'remote_write_file', 'ssh_tunnel',
    'list_custom_tools',
}


class ToolExecutor:
    def __init__(self, permissions, renderer, cwd: str, process_manager=None):
        self.permissions = permissions
        self.renderer = renderer
        self.cwd = cwd
        self.process_manager = process_manager

    async def execute(self, name: str, input: dict) -> "dict | None":
        """Execute a tool locally. Returns None to signal server-side fallback."""
        if name in SERVER_TOOLS or name not in LOCAL_TOOLS:
            return None

        if not self.permissions.is_auto_approved(name, input):
            approved = await self.permissions.prompt(name, input)
            if not approved:
                return {'error': 'Denied by user'}

        if name == 'read_file':
            return file_ops.read_file(input, self.cwd)
        elif name == 'write_file':
            return file_ops.write_file(input, self.cwd)
        elif name == 'edit_file':
            return file_ops.edit_file(input, self.cwd)
        elif name == 'exec':
            return await shell.execute(input, self.cwd, process_manager=self.process_manager)
        elif name == 'glob':
            return search.glob_search(input, self.cwd)
        elif name == 'grep':
            return search.grep_search(input, self.cwd)
        elif name == 'web_fetch':
            return None  # delegate to server for now
        else:
            return None
