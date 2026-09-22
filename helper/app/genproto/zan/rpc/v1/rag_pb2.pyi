from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class SearchRequest(_message.Message):
    __slots__ = ("query_text", "top_k")
    QUERY_TEXT_FIELD_NUMBER: _ClassVar[int]
    TOP_K_FIELD_NUMBER: _ClassVar[int]
    query_text: str
    top_k: int
    def __init__(self, query_text: _Optional[str] = ..., top_k: _Optional[int] = ...) -> None: ...

class SearchMatch(_message.Message):
    __slots__ = ("ref", "quote", "score")
    REF_FIELD_NUMBER: _ClassVar[int]
    QUOTE_FIELD_NUMBER: _ClassVar[int]
    SCORE_FIELD_NUMBER: _ClassVar[int]
    ref: str
    quote: str
    score: float
    def __init__(self, ref: _Optional[str] = ..., quote: _Optional[str] = ..., score: _Optional[float] = ...) -> None: ...

class SearchResponse(_message.Message):
    __slots__ = ("matches",)
    MATCHES_FIELD_NUMBER: _ClassVar[int]
    matches: _containers.RepeatedCompositeFieldContainer[SearchMatch]
    def __init__(self, matches: _Optional[_Iterable[_Union[SearchMatch, _Mapping]]] = ...) -> None: ...
