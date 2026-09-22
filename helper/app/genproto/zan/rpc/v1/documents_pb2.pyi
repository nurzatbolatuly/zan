from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class RenderFormat(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    RENDER_FORMAT_UNSPECIFIED: _ClassVar[RenderFormat]
    RENDER_FORMAT_PDF: _ClassVar[RenderFormat]
    RENDER_FORMAT_DOCX: _ClassVar[RenderFormat]
RENDER_FORMAT_UNSPECIFIED: RenderFormat
RENDER_FORMAT_PDF: RenderFormat
RENDER_FORMAT_DOCX: RenderFormat

class DocumentSection(_message.Message):
    __slots__ = ("title", "body")
    TITLE_FIELD_NUMBER: _ClassVar[int]
    BODY_FIELD_NUMBER: _ClassVar[int]
    title: str
    body: str
    def __init__(self, title: _Optional[str] = ..., body: _Optional[str] = ...) -> None: ...

class RenderRequest(_message.Message):
    __slots__ = ("title", "sections", "format")
    TITLE_FIELD_NUMBER: _ClassVar[int]
    SECTIONS_FIELD_NUMBER: _ClassVar[int]
    FORMAT_FIELD_NUMBER: _ClassVar[int]
    title: str
    sections: _containers.RepeatedCompositeFieldContainer[DocumentSection]
    format: RenderFormat
    def __init__(self, title: _Optional[str] = ..., sections: _Optional[_Iterable[_Union[DocumentSection, _Mapping]]] = ..., format: _Optional[_Union[RenderFormat, str]] = ...) -> None: ...

class RenderResponse(_message.Message):
    __slots__ = ("file_url", "object_key")
    FILE_URL_FIELD_NUMBER: _ClassVar[int]
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    file_url: str
    object_key: str
    def __init__(self, file_url: _Optional[str] = ..., object_key: _Optional[str] = ...) -> None: ...
