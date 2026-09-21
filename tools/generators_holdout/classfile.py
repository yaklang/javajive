"""Minimal class-file reader: this_class name and enclosing type.

Used to distinguish a legal top-level `$` identifier from a nested binary name.
Invalid/truncated bytes return kind=invalid; callers must not guess.
"""

from __future__ import annotations

import struct
from typing import Any


class ClassFileError(ValueError):
    pass


def _u1(data: bytes, off: int) -> tuple[int, int]:
    if off >= len(data):
        raise ClassFileError("truncated")
    return data[off], off + 1


def _u2(data: bytes, off: int) -> tuple[int, int]:
    if off + 2 > len(data):
        raise ClassFileError("truncated")
    return struct.unpack_from(">H", data, off)[0], off + 2


def _u4(data: bytes, off: int) -> tuple[int, int]:
    if off + 4 > len(data):
        raise ClassFileError("truncated")
    return struct.unpack_from(">I", data, off)[0], off + 4


def _parse_cp(data: bytes, off: int, count: int) -> tuple[list[Any], int]:
    pool: list[Any] = [None]
    i = 1
    while i < count:
        tag, off = _u1(data, off)
        if tag == 1:
            ln, off = _u2(data, off)
            if off + ln > len(data):
                raise ClassFileError("truncated utf8")
            pool.append(("utf8", data[off : off + ln].decode("utf-8", "replace")))
            off += ln
        elif tag in {7, 8, 16, 19, 20}:
            idx, off = _u2(data, off)
            pool.append(("idx", tag, idx))
        elif tag in {3, 4}:
            _, off = _u4(data, off)
            pool.append(("u4", tag))
        elif tag in {5, 6}:
            if off + 8 > len(data):
                raise ClassFileError("truncated wide")
            off += 8
            pool.append(("wide", tag))
            pool.append(None)
            i += 1
        elif tag in {9, 10, 11, 12, 17, 18}:
            _, off = _u2(data, off)
            _, off = _u2(data, off)
            pool.append(("pair", tag))
        elif tag == 15:
            if off + 3 > len(data):
                raise ClassFileError("truncated methodhandle")
            off += 3
            pool.append(("mh",))
        else:
            raise ClassFileError(f"unknown cp tag {tag}")
        i += 1
    return pool, off


def _cp_utf8(pool: list[Any], idx: int) -> str | None:
    if idx <= 0 or idx >= len(pool) or pool[idx] is None:
        return None
    ent = pool[idx]
    if ent[0] == "utf8":
        return ent[1]
    if ent[0] == "idx" and ent[1] == 7:
        return _cp_utf8(pool, ent[2])
    return None


def _skip_members(data: bytes, off: int, n: int) -> int:
    for _ in range(n):
        off += 6
        ac, off = _u2(data, off)
        for _a in range(ac):
            off += 2
            ln, off = _u4(data, off)
            off += ln
    return off


def _inner_outer(pool: list[Any], body: bytes, this_class_idx: int) -> str | None:
    if len(body) < 2:
        return None
    n = struct.unpack_from(">H", body, 0)[0]
    pos = 2
    for _ in range(n):
        if pos + 8 > len(body):
            return None
        inner, outer, _iname, _flags = struct.unpack_from(">HHHH", body, pos)
        pos += 8
        if inner == this_class_idx and outer != 0:
            name = _cp_utf8(pool, outer)
            if name:
                return name.replace("/", ".")
    return None


def class_enclosing_info(class_bytes: bytes) -> dict[str, Any]:
    """Return this_name, enclosing, kind.

    kind is top_level | nested_member | nested_local | invalid.
    enclosing is the outer/enclosing binary name when nested.
    """
    try:
        return _parse(class_bytes)
    except (ClassFileError, struct.error, UnicodeError, IndexError, TypeError):
        return {"this_name": None, "enclosing": None, "kind": "invalid"}


def _parse(data: bytes) -> dict[str, Any]:
    if len(data) < 10 or data[:4] != b"\xca\xfe\xba\xbe":
        raise ClassFileError("not a class")
    off = 8
    cp_count, off = _u2(data, off)
    pool, off = _parse_cp(data, off, cp_count)
    _access, off = _u2(data, off)
    this_idx, off = _u2(data, off)
    _super, off = _u2(data, off)
    ifc_count, off = _u2(data, off)
    off += 2 * ifc_count
    nfields, off = _u2(data, off)
    off = _skip_members(data, off, nfields)
    nmethods, off = _u2(data, off)
    off = _skip_members(data, off, nmethods)
    nattr, off = _u2(data, off)
    this_internal = _cp_utf8(pool, this_idx)
    this_name = this_internal.replace("/", ".") if this_internal else None
    enclosing_method: str | None = None
    inner_outer: str | None = None
    for _ in range(nattr):
        name_idx, off = _u2(data, off)
        ln, off = _u4(data, off)
        body = data[off : off + ln]
        off += ln
        aname = _cp_utf8(pool, name_idx)
        if aname == "InnerClasses":
            inner_outer = _inner_outer(pool, body, this_idx)
        elif aname == "EnclosingMethod" and len(body) >= 2:
            enc_idx = struct.unpack_from(">H", body, 0)[0]
            enc = _cp_utf8(pool, enc_idx)
            if enc:
                enclosing_method = enc.replace("/", ".")
    if inner_outer:
        return {"this_name": this_name, "enclosing": inner_outer, "kind": "nested_member"}
    if enclosing_method:
        return {"this_name": this_name, "enclosing": enclosing_method, "kind": "nested_local"}
    return {"this_name": this_name, "enclosing": None, "kind": "top_level"}
