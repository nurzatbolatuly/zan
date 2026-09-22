from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional

DESCRIPTOR: _descriptor.FileDescriptor

class Lang(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    LANG_UNSPECIFIED: _ClassVar[Lang]
    LANG_RU: _ClassVar[Lang]
    LANG_KZ: _ClassVar[Lang]
LANG_UNSPECIFIED: Lang
LANG_RU: Lang
LANG_KZ: Lang

class TraceContext(_message.Message):
    __slots__ = ("trace_id", "session_id")
    TRACE_ID_FIELD_NUMBER: _ClassVar[int]
    SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    trace_id: str
    session_id: str
    def __init__(self, trace_id: _Optional[str] = ..., session_id: _Optional[str] = ...) -> None: ...
