"""Кладёт app/genproto на sys.path, до любого импорта сгенерированного кода.

Сгенерированный protobuf/gRPC-код (`grpc_tools.protoc`, см. Makefile и
BACKEND_PLAN.md §3.2) импортирует друг друга по пути, зафиксированному в
самих `.proto` (например, `from zan.rpc.v1 import common_pb2`), и ничего не
знает про нашу обёрточную директорию `app/genproto/`. Без этого шага такой
импорт ломается, как только у одного `.proto` появляется зависимость от
другого — уже происходит начиная со Stage 4 (`stt.proto` импортирует
`common.proto`).

Единообразие обязательно: и наш собственный код, и cross-imports между
сгенерированными файлами обращаются к пакету как `zan.rpc.v1.xxx_pb2` —
никогда как `app.genproto.zan.rpc.v1.xxx_pb2`. Два разных пути импорта
одного и того же модуля создали бы два разных объекта дескриптора в
protobuf'овском descriptor pool и падали бы с ошибкой дублирования.

Пакет назывался `zan.internal.v1` до Stage 4 — переименован в `zan.rpc.v1`
(причина — Go-специфичная, никак не про Python: см. proto/zan/rpc/v1/common.proto
и BACKEND_LOG.md, Stage 4).

Импортируется первой строкой в app/main.py и в tests/conftest.py — раньше
любого `from zan.rpc.v1 import ...`.
"""

import sys
from pathlib import Path

_GENPROTO_DIR = Path(__file__).resolve().parent / "genproto"

if str(_GENPROTO_DIR) not in sys.path:
    sys.path.insert(0, str(_GENPROTO_DIR))
