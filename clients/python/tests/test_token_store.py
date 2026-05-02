import base64
import json
import time

import pytest

from queueti._token_store import TokenStore, parse_token_expiry, seconds_until_expiry


def _make_jwt(exp: float) -> str:
    header = base64.urlsafe_b64encode(b'{"alg":"HS256"}').decode().rstrip("=")
    payload = base64.urlsafe_b64encode(
        json.dumps({"exp": exp}).encode()
    ).decode().rstrip("=")
    return f"{header}.{payload}.fakesig"


class TestTokenStore:
    def test_get_returns_initial_token(self) -> None:
        store = TokenStore("tok-abc")
        assert store.get() == "tok-abc"

    def test_set_updates_token(self) -> None:
        store = TokenStore("old")
        store.set("new")
        assert store.get() == "new"


class TestParseTokenExpiry:
    def test_returns_exp_from_valid_jwt(self) -> None:
        exp = 9999999999.0
        token = _make_jwt(exp)
        assert parse_token_expiry(token) == exp

    def test_raises_for_non_jwt(self) -> None:
        with pytest.raises(ValueError, match="not a valid JWT"):
            parse_token_expiry("notajwt")

    def test_raises_when_exp_missing(self) -> None:
        header = base64.urlsafe_b64encode(b'{"alg":"HS256"}').decode().rstrip("=")
        payload = base64.urlsafe_b64encode(b'{"sub":"user"}').decode().rstrip("=")
        token = f"{header}.{payload}.fakesig"
        with pytest.raises(ValueError, match="no exp claim"):
            parse_token_expiry(token)


class TestSecondsUntilExpiry:
    def test_returns_positive_when_not_expired(self) -> None:
        future_exp = time.time() + 3600
        token = _make_jwt(future_exp)
        secs = seconds_until_expiry(token, advance_seconds=0)
        assert 3590 < secs < 3610

    def test_returns_negative_when_expired(self) -> None:
        past_exp = time.time() - 100
        token = _make_jwt(past_exp)
        assert seconds_until_expiry(token, advance_seconds=0) < 0

    def test_advance_window_reduces_remaining(self) -> None:
        future_exp = time.time() + 3600
        token = _make_jwt(future_exp)
        without_advance = seconds_until_expiry(token, advance_seconds=0)
        with_advance = seconds_until_expiry(token, advance_seconds=60)
        assert abs((without_advance - with_advance) - 60) < 1
