"""Trace the existing pq Driver.Open path; replace only AI providers."""
import json
import pathlib
import sys

root, output = map(pathlib.Path, sys.argv[1:3])
output.mkdir(parents=True, exist_ok=True)
source = root / "pkgs/store/storage/sqlx.go"
text = source.read_text()
needle = "db, err := sqlx.Open(driver, dsn)"
assert text.count(needle) == 1
replacement = output / "sqlx.go"
replacement.write_text(text.replace(needle, '''openDriver := driver
	if driver == "postgres" { openDriver = "run-state-trace" }
	db, err := sqlx.Open(openDriver, dsn)
	if db != nil && driver == "postgres" { db = sqlx.NewDb(db.DB, driver) }'''))
service = root / "pkgs/gizclaw/services/ai/peergenx/service.go"
text = service.read_text()
assert text.count("DefaultBuilder{}") == 2
provider = output / "service.go"
provider.write_text(text.replace("DefaultBuilder{}", "runStateFixtureBuilder{}"))
(output / "overlay.json").write_text(json.dumps({"Replace": {
    str(source): str(replacement),
    str(service): str(provider),
    str(service.parent / "run_state_fixture.go"): str(root / "tests/gizclaw-e2e/testdata/run-state-postgres/provider.go"),
}}))
