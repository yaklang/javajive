"""Typed ledger errors. Missing required items are errors, not skips."""

from __future__ import annotations

from typing import Any


class LedgerError(Exception):
    def __init__(
        self,
        message: str,
        *,
        code: str = "ledger_error",
        missing_ids: list[str] | None = None,
        duplicate_ids: list[str] | None = None,
        unexpected_ids: list[str] | None = None,
        details: dict[str, Any] | None = None,
    ) -> None:
        super().__init__(message)
        self.code = code
        self.missing_ids = list(missing_ids or [])
        self.duplicate_ids = list(duplicate_ids or [])
        self.unexpected_ids = list(unexpected_ids or [])
        self.details = dict(details or {})

    def named_ids(self) -> list[str]:
        return list(self.missing_ids) + list(self.duplicate_ids) + list(self.unexpected_ids)


class ObservationError(LedgerError):
    def __init__(self, message: str, **kwargs: Any) -> None:
        kwargs.setdefault("code", "invalid_observation")
        super().__init__(message, **kwargs)


class MissingEvidenceError(ObservationError):
    def __init__(self, message: str, **kwargs: Any) -> None:
        kwargs.setdefault("code", "missing_evidence")
        super().__init__(message, **kwargs)


class CorruptJSONError(LedgerError):
    def __init__(self, message: str, **kwargs: Any) -> None:
        kwargs.setdefault("code", "corrupt_json")
        super().__init__(message, **kwargs)


class InventoryError(LedgerError):
    def __init__(self, message: str, **kwargs: Any) -> None:
        kwargs.setdefault("code", "inventory")
        super().__init__(message, **kwargs)


class PromotionError(LedgerError):
    def __init__(self, message: str, **kwargs: Any) -> None:
        kwargs.setdefault("code", "promotion")
        super().__init__(message, **kwargs)


class ExpectationChangeError(LedgerError):
    def __init__(self, message: str, **kwargs: Any) -> None:
        kwargs.setdefault("code", "expectation_change")
        super().__init__(message, **kwargs)
