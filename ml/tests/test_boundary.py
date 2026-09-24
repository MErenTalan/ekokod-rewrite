"""R366 / 03 §6.2: the ML service has no database driver — not installed, not locked, not imported."""

import re
from pathlib import Path

from ekokod_ml.nodb import FORBIDDEN, database_clients

ROOT = Path(__file__).resolve().parents[1]


def test_no_installed_distribution_is_a_database_client():
    assert database_clients() == []


def test_the_lockfile_holds_no_database_client_in_any_group_or_extra():
    names = re.findall(r'^name = "([^"]+)"', (ROOT / "uv.lock").read_text(), flags=re.M)
    assert names, "uv.lock parsed"
    assert [n for n in names if FORBIDDEN.match(n)] == []


def test_the_detector_would_catch_one():
    assert database_clients(["psycopg-binary", "numpy", "SQLAlchemy", "redis", "asyncpg"]) == ["psycopg-binary", "SQLAlchemy", "redis", "asyncpg"]


def test_no_source_file_imports_a_database_client():
    src = "\n".join(p.read_text() for p in (ROOT / "src").rglob("*.py"))
    assert not re.search(r"^\s*(import|from)\s+(psycopg|pymongo|sqlalchemy|asyncpg|pymysql|MySQLdb|redis|motor)\b", src, flags=re.M)
