"""Slash command registry."""

_commands = {}


def command(*names):
    def decorator(fn):
        for name in names:
            _commands[name] = fn
        return fn
    return decorator


def get_command(name: str):
    return _commands.get(name)


def all_commands():
    return dict(_commands)
