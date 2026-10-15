package sqlitero

// Tables lists the database's tables.
func (db *DB) Tables() []string {
	var out []string
	for _, s := range db.schema {
		if s.typ == "table" {
			out = append(out, s.name)
		}
	}
	return out
}
