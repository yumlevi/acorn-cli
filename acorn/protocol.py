"""WebSocket message protocol types."""

import json


def chat_message(session_id: str, content: str, user_name: str) -> str:
    return json.dumps({
        'type': 'chat',
        'content': content,
        'sessionId': session_id,
        'userName': user_name,
    })


def tool_result_message(tool_id: str, result) -> str:
    return json.dumps({
        'type': 'tool:result',
        'id': tool_id,
        'result': result,
    })


def stop_message(session_id: str = None) -> str:
    msg = {'type': 'chat:stop'}
    if session_id:
        msg['sessionId'] = session_id
    return json.dumps(msg)


def clear_message(session_id: str = None) -> str:
    msg = {'type': 'chat:clear'}
    if session_id:
        msg['sessionId'] = session_id
    return json.dumps(msg)
