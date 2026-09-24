"""R366: prove the image carries no database client. `python -m ekokod_ml.nodb` exits 1 if it finds one."""

import re
import sys
from importlib.metadata import distributions

FORBIDDEN = re.compile(
    r"^(psycopg.*|pymongo|motor|sqlalchemy|asyncpg|pymysql|mysqlclient|mysql-connector.*|redis|cassandra-driver|clickhouse.*|pyodbc|cx[-_]oracle|oracledb|aiopg|databases)$",
    re.IGNORECASE,
)


def database_clients(names: list[str] | None = None) -> list[str]:
    names = names if names is not None else [d.metadata["Name"] for d in distributions()]
    return [n for n in names if FORBIDDEN.match(n)]


if __name__ == "__main__":
    found = database_clients()
    print("database clients:", ", ".join(found) if found else "none")
    sys.exit(1 if found else 0)
