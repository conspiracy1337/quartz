# Regenerates the Modrinth App databases: python3 gen.py
import os, sqlite3

def fresh(name):
    if os.path.exists(name):
        os.remove(name)
    return sqlite3.connect(name)

# The older schema: one profiles table.
db = fresh("modrinth-old.db")
db.execute("""CREATE TABLE profiles (path TEXT NOT NULL PRIMARY KEY, install_stage TEXT NOT NULL, name TEXT NOT NULL,
  icon_path TEXT NULL, game_version TEXT NOT NULL, mod_loader TEXT NOT NULL, mod_loader_version TEXT NULL,
  last_played INTEGER NULL, submitted_time_played INTEGER NOT NULL DEFAULT 0, recent_time_played INTEGER NOT NULL DEFAULT 0,
  override_mc_memory_max INTEGER NULL)""")
db.execute("CREATE TABLE settings (id INTEGER PRIMARY KEY, custom_dir TEXT NULL)")
db.execute("INSERT INTO settings VALUES (0, NULL)")
db.execute("INSERT INTO profiles VALUES ('fo', 'installed', 'Fabulously Optimized', NULL, '1.21.4', 'fabric', '0.16.10', 1760000000, 3000, 600, 6144)")
db.execute("INSERT INTO profiles VALUES ('../escape', 'installed', 'Bad', NULL, '1.21.4', 'vanilla', NULL, NULL, 0, 0, NULL)")
db.execute("INSERT INTO profiles VALUES ('lite', 'installed', 'Lite', NULL, '1.12.2', 'liteloader', NULL, NULL, 0, 0, NULL)")
db.commit(); db.close()

# The newer schema: instances and their content sets.
db = fresh("modrinth-new.db")
db.execute("CREATE TABLE instance_content_sets (id INTEGER PRIMARY KEY, game_version TEXT NOT NULL, loader TEXT NOT NULL, loader_version TEXT NULL)")
db.execute("CREATE TABLE instances (path TEXT NOT NULL PRIMARY KEY, name TEXT NOT NULL, icon_path TEXT NULL, applied_content_set_id INTEGER NULL)")
db.execute("INSERT INTO instance_content_sets VALUES (7, '1.20.1', 'neoforge', '47.1.106')")
db.execute("INSERT INTO instances VALUES ('pack', 'All the Mods', NULL, 7)")
db.commit(); db.close()
