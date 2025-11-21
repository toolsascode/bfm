# SFM Examples for the BFM Solution

These examples illustrate how to structure migrations for the Backend For Migrations (BFM) solution across every supported backend. They are intentionally small and self-contained so that teams can copy them into `sfm/` and expand on them for real environments.

## Included Samples

- `postgresql/solution/core_solution_*`: Fixed-schema example for the Core backend that creates a `solution_runs` table with versioning metadata.
- `greptimedb/solution/logs_solution_*`: Dynamic-schema example for the Logs backend that stores streaming observability metrics.
- `etcd/solution/metadata_solution_*`: Metadata example that seeds global feature toggles in Etcd.

Each backend includes:

1. A Go file that registers the migration via `bfm/api/migrations`.
2. An `Up` payload (`.sql` or `.json`).
3. A `Down` payload that safely rolls the change back.

## Usage

1. Copy one of the example folders into `sfm/`.
2. Rename the files following `{schema}_{table}_{version}_{name}`.
3. Update the SQL/JSON payloads with your real schema definitions.
4. Rebuild or restart the BFM server so it picks up the new scripts.

The samples are meant to stay lightweight so contributors can understand the solution without sifting through production data dumps.
