# Regenerates the test databases with Python's sqlite3: python3 gen.py
import os, shutil, sqlite3

def fresh(name):
    for suffix in ("", "-wal", "-shm", "-journal"):
        if os.path.exists(name + suffix):
            os.remove(name + suffix)
    return sqlite3.connect(name)

# basic.db: 1 KiB pages so 500 rows need interior pages, one row spills onto
# overflow pages, a column added later, quoted names, negative numbers.
db = fresh("basic.db")
db.execute("PRAGMA page_size=1024")
db.execute("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT NOT NULL, n INTEGER, f REAL, b BLOB, neg INTEGER)")
for i in range(1, 501):
    db.execute("INSERT INTO t VALUES (?,?,?,?,?,?)", (i, "row %d" % i, i * 1000, i / 4, bytes([i % 256, 1, 2]), -i * 70000))
db.execute("INSERT INTO t VALUES (?,?,?,?,?,?)", (1000, "x" * 5000, 2**40, None, None, -1))
db.execute("ALTER TABLE t ADD COLUMN extra TEXT DEFAULT 'd'")
db.execute("INSERT INTO t (id, name, extra) VALUES (1001, 'late', 'here')")
db.execute('CREATE TABLE "my table" ("col one" TEXT, [col2] INT, CONSTRAINT pk PRIMARY KEY ("col one"))')
db.execute('INSERT INTO "my table" VALUES (?, ?)', ("ünïcödé", 7))
db.commit()
db.close()

# wal.db: rows that live only in the write-ahead log.
db = fresh("wal-src.db")
db.execute("PRAGMA journal_mode=WAL")
db.execute("PRAGMA wal_autocheckpoint=0")
db.execute("CREATE TABLE profiles (path TEXT, game_version TEXT)")
db.execute("INSERT INTO profiles VALUES ('one', '1.20.1')")
db.commit()
db.execute("INSERT INTO profiles VALUES ('two', '1.21.4')")
db.commit()
shutil.copy("wal-src.db", "wal.db")
shutil.copy("wal-src.db-wal", "wal.db-wal")
db.close()
for suffix in ("", "-wal", "-shm"):
    if os.path.exists("wal-src.db" + suffix):
        os.remove("wal-src.db" + suffix)

# utf16.db
db = fresh("utf16.db")
db.execute("PRAGMA encoding='UTF-16le'")
db.execute("CREATE TABLE s (v TEXT)")
db.execute("INSERT INTO s VALUES ('Grüße')")
db.commit()
db.close()
