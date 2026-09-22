from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional

DESCRIPTOR: _descriptor.FileDescriptor

class ExtractRequest(_message.Message):
    __slots__ = ("file_url", "mime_type")
    FILE_URL_FIELD_NUMBER: _ClassVar[int]
    MIME_TYPE_FIELD_NUMBER: _ClassVar[int]
    file_url: str
    mime_type: str
    def __init__(self, file_url: _Optional[str] = ..., mime_type: _Optional[str] = ...) -> None: ...

class ExtractResponse(_message.Message):
    __slots__ = ("text",)
    TEXT_FIELD_NUMBER: _ClassVar[int]
    text: str
    def __init__(self, text: _Optional[str] = ...) -> None: ...
