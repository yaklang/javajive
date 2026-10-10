"""Bounded class-file metadata reader for compilation-unit classification.

This validates structural bounds and typed constant-pool references; it is not a
bytecode verifier. Invalid metadata never becomes a guessed top-level class.
"""
from __future__ import annotations

import struct
from typing import Any


class ClassFileError(ValueError):
    pass


class _Reader:
    def __init__(self, data: bytes):
        self.data, self.off = data, 0

    def take(self, n: int) -> bytes:
        if n < 0 or n > len(self.data) - self.off:
            raise ClassFileError("truncated region")
        start = self.off
        self.off += n
        return self.data[start:self.off]

    def u1(self) -> int:
        return self.take(1)[0]

    def u2(self) -> int:
        return struct.unpack(">H", self.take(2))[0]

    def u4(self) -> int:
        return struct.unpack(">I", self.take(4))[0]

    def finish(self) -> None:
        if self.off != len(self.data):
            raise ClassFileError("unconsumed region bytes")


def _mutf8(data: bytes) -> str:
    units = bytearray()
    r = _Reader(data)
    while r.off < len(data):
        first = r.u1()
        if 1 <= first <= 0x7f:
            value = first
        elif 0xc0 <= first <= 0xdf:
            second = r.u1()
            if second & 0xc0 != 0x80:
                raise ClassFileError("invalid modified UTF-8 continuation")
            value = ((first & 0x1f) << 6) | (second & 0x3f)
            if value < 0x80 and value != 0:
                raise ClassFileError("overlong modified UTF-8")
        elif 0xe0 <= first <= 0xef:
            second, third = r.u1(), r.u1()
            if second & 0xc0 != 0x80 or third & 0xc0 != 0x80:
                raise ClassFileError("invalid modified UTF-8 continuation")
            value = ((first & 15) << 12) | ((second & 63) << 6) | (third & 63)
            if value < 0x800:
                raise ClassFileError("overlong modified UTF-8")
        else:
            raise ClassFileError("invalid modified UTF-8 lead byte")
        units.extend(struct.pack(">H", value))
    return units.decode("utf-16-be", "surrogatepass")


def _entry(pool: list[Any], idx: int, tags: set[int]) -> tuple:
    if not 0 < idx < len(pool) or pool[idx] is None or pool[idx][0] not in tags:
        raise ClassFileError(f"invalid constant-pool reference {idx}, expected {sorted(tags)}")
    return pool[idx]


def _cp_utf8(pool: list[Any], idx: int) -> str:
    # CONSTANT_Class.name_index must point directly to Utf8, never another
    # Class. No recursion means self-cycles and arbitrarily long chains fail.
    return _entry(pool, idx, {1})[1]


def _cp_class(pool: list[Any], idx: int) -> str:
    name = _cp_utf8(pool, _entry(pool, idx, {7})[1])
    if not name or any(c in name for c in ".;[\x00") or any(not part for part in name.split("/")):
        raise ClassFileError("invalid class name")
    return name


def _parse_cp(r: _Reader) -> list[Any]:
    count = r.u2()
    if count == 0:
        raise ClassFileError("zero constant-pool count")
    pool: list[Any] = [None]
    while len(pool) < count:
        tag = r.u1()
        if tag == 1:
            pool.append((tag, _mutf8(r.take(r.u2()))))
        elif tag in {7, 8, 16, 19, 20}:
            pool.append((tag, r.u2()))
        elif tag in {3, 4}:
            pool.append((tag, r.take(4)))
        elif tag in {5, 6}:
            if len(pool) + 1 >= count:
                raise ClassFileError("wide constant missing reserved slot")
            pool.extend([(tag, r.take(8)), None])
        elif tag in {9, 10, 11, 12, 17, 18}:
            pool.append((tag, r.u2(), r.u2()))
        elif tag == 15:
            pool.append((tag, r.u1(), r.u2()))
        else:
            raise ClassFileError(f"unknown constant-pool tag {tag}")
    for ent in pool[1:]:
        if ent is None:
            continue
        tag = ent[0]
        if tag in {7, 8, 16, 19, 20}:
            _cp_utf8(pool, ent[1])
        elif tag in {9, 10, 11}:
            _entry(pool, ent[1], {7})
            _entry(pool, ent[2], {12})
        elif tag == 12:
            _cp_utf8(pool, ent[1])
            _cp_utf8(pool, ent[2])
        elif tag in {17, 18}:
            _entry(pool, ent[2], {12})
        elif tag == 15:
            kind = ent[1]
            allowed = {9} if kind in {1, 2, 3, 4} else {10} if kind in {5, 8} else {10, 11} if kind in {6, 7} else {11} if kind == 9 else set()
            _entry(pool, ent[2], allowed)
    return pool


def _attributes(r: _Reader, pool: list[Any]):
    for _ in range(r.u2()):
        name = _cp_utf8(pool, r.u2())
        yield name, r.take(r.u4())


def _skip_members(r: _Reader, pool: list[Any]) -> None:
    for _ in range(r.u2()):
        r.u2()  # access
        _cp_utf8(pool, r.u2())
        _cp_utf8(pool, r.u2())
        for _name, _body in _attributes(r, pool):
            pass


def class_enclosing_info(class_bytes: bytes) -> dict[str, Any]:
    try:
        return _parse(class_bytes)
    except (ClassFileError, struct.error, UnicodeError, IndexError, TypeError) as exc:
        return {"this_name": None, "enclosing": None, "kind": "invalid", "error": str(exc)}


def _parse(data: bytes) -> dict[str, Any]:
    r = _Reader(data)
    if r.u4() != 0xcafebabe:
        raise ClassFileError("not a class")
    r.take(4)  # minor, major
    pool = _parse_cp(r)
    r.u2()  # access
    this_idx, super_idx = r.u2(), r.u2()
    this_internal = _cp_class(pool, this_idx)
    if super_idx:
        _cp_class(pool, super_idx)
    for _ in range(r.u2()):
        _cp_class(pool, r.u2())
    _skip_members(r, pool)
    _skip_members(r, pool)
    enclosing_method = inner_outer = None
    members: list[str] = []
    parents: dict[str, str] = {}
    seen = set()
    for aname, data in _attributes(r, pool):
        body = _Reader(data)
        if aname in {"InnerClasses", "EnclosingMethod"}:
            if aname in seen:
                raise ClassFileError(f"duplicate {aname}")
            seen.add(aname)
        if aname == "InnerClasses":
            for _ in range(body.u2()):
                inner, outer, inner_name, _flags = (body.u2() for _ in range(4))
                inner_class = _cp_class(pool, inner)
                outer_class = _cp_class(pool, outer) if outer else None
                if inner_name:
                    _cp_utf8(pool, inner_name)
                if outer_class:
                    if inner_class == outer_class or inner_class in parents and parents[inner_class] != outer_class:
                        raise ClassFileError("invalid enclosing-class relation")
                    parents[inner_class] = outer_class
                if inner == this_idx and outer:
                    if inner_outer is not None:
                        raise ClassFileError("duplicate inner-class identity")
                    inner_outer = outer_class
                if outer == this_idx:
                    members.append(inner_class.replace("/", "."))
            body.finish()
        elif aname == "EnclosingMethod":
            enclosing_method = _cp_class(pool, body.u2())
            method_idx = body.u2()
            if method_idx:
                _entry(pool, method_idx, {12})
            body.finish()
    r.finish()
    if inner_outer and enclosing_method and inner_outer != enclosing_method:
        raise ClassFileError("conflicting enclosing metadata")
    enclosing = inner_outer or enclosing_method
    if enclosing == this_internal:
        raise ClassFileError("self-enclosing class")
    # Follow available InnerClasses links iteratively; no `$`-name guesses.
    unit = enclosing or this_internal
    visited = {this_internal} if enclosing else set()
    while unit in parents:
        if unit in visited:
            raise ClassFileError("cyclic enclosing-class relation")
        visited.add(unit)
        unit = parents[unit]
    if enclosing and unit == this_internal:
        raise ClassFileError("cyclic enclosing-class relation")
    return {
        "this_name": this_internal.replace("/", "."),
        "enclosing": enclosing.replace("/", ".") if enclosing else None,
        "kind": "nested_member" if inner_outer else "nested_local" if enclosing_method else "top_level",
        "declared_members": members,
        "compilation_unit": unit.replace("/", "."),
    }
