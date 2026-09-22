from zan.rpc.v1 import common_pb2 as _common_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class TranscribeRequest(_message.Message):
    __slots__ = ("file_url", "mime_type", "lang")
    FILE_URL_FIELD_NUMBER: _ClassVar[int]
    MIME_TYPE_FIELD_NUMBER: _ClassVar[int]
    LANG_FIELD_NUMBER: _ClassVar[int]
    file_url: str
    mime_type: str
    lang: _common_pb2.Lang
    def __init__(self, file_url: _Optional[str] = ..., mime_type: _Optional[str] = ..., lang: _Optional[_Union[_common_pb2.Lang, str]] = ...) -> None: ...

class TranscribeResponse(_message.Message):
    __slots__ = ("text",)
    TEXT_FIELD_NUMBER: _ClassVar[int]
    text: str
    def __init__(self, text: _Optional[str] = ...) -> None: ...
