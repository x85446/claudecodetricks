"""
PM Review Webtool — FastAPI backend for the product management database.

Launch:
    python webtool/serve.py --project ~/workspace/izuma/myriplay

Serves the review UI and exposes a REST API over the marketing.sqlite database.
"""

from __future__ import annotations

import argparse
import sqlite3
import webbrowser
from contextlib import contextmanager
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import JSONResponse
from fastapi.staticfiles import StaticFiles

import uvicorn

# ---------------------------------------------------------------------------
# App setup
# ---------------------------------------------------------------------------

app = FastAPI(title="PM Review Tool")

# Resolved at startup via CLI args
DB_PATH: Path = Path()
STATIC_DIR: Path = Path(__file__).resolve().parent / "static"


# ---------------------------------------------------------------------------
# Database helpers
# ---------------------------------------------------------------------------

@contextmanager
def get_db():
    """Yield a configured SQLite connection with row factory."""
    conn = sqlite3.connect(str(DB_PATH))
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA foreign_keys=ON")
    conn.execute("PRAGMA journal_mode=WAL")
    try:
        yield conn
    finally:
        conn.close()


def rows_to_dicts(rows: list[sqlite3.Row]) -> list[dict[str, Any]]:
    return [dict(r) for r in rows]


def row_to_dict(row: sqlite3.Row | None) -> dict[str, Any] | None:
    return dict(row) if row else None


def _ensure_feedback_table(conn: sqlite3.Connection) -> None:
    conn.execute("""
        CREATE TABLE IF NOT EXISTS feedback (
            id          INTEGER PRIMARY KEY AUTOINCREMENT,
            product_id  INTEGER NOT NULL REFERENCES our_products(id),
            entity_type TEXT NOT NULL,
            entity_id   TEXT NOT NULL,
            feedback_text TEXT NOT NULL,
            created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
        )
    """)


def _now() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S")


# ---------------------------------------------------------------------------
# GET /api/products
# ---------------------------------------------------------------------------

@app.get("/api/products")
def list_products():
    with get_db() as conn:
        rows = conn.execute("SELECT id, code, name FROM our_products ORDER BY name").fetchall()
        return rows_to_dicts(rows)


# ---------------------------------------------------------------------------
# GET /api/{product_code}/tree
# ---------------------------------------------------------------------------

@app.get("/api/{product_code}/tree")
def product_tree(product_code: str):
    with get_db() as conn:
        pid_row = conn.execute(
            "SELECT id FROM our_products WHERE code = ?", (product_code,)
        ).fetchone()
        if not pid_row:
            raise HTTPException(404, f"Product '{product_code}' not found")
        pid = pid_row["id"]

        # Epics
        epics = rows_to_dicts(conn.execute(
            """SELECT id, name, description, rationale, version, base_version,
                      human_approved, status, source
               FROM epics WHERE product_id = ? ORDER BY name""",
            (pid,),
        ).fetchall())

        # Features keyed by epic_id
        features = rows_to_dicts(conn.execute(
            """SELECT id, epic_id, short_desc, detailed_desc, version, base_version,
                      human_approved, status, target_release, source
               FROM product_features WHERE product_id = ? ORDER BY short_desc""",
            (pid,),
        ).fetchall())

        # Requirements keyed by feature_id
        feature_ids = [f["id"] for f in features]
        requirements: list[dict] = []
        if feature_ids:
            placeholders = ",".join("?" * len(feature_ids))
            requirements = rows_to_dicts(conn.execute(
                f"""SELECT id, feature_id, title, description, acceptance_criteria,
                           version, base_version, human_approved, status, source
                    FROM requirements WHERE feature_id IN ({placeholders})
                    ORDER BY title""",
                feature_ids,
            ).fetchall())

        # Tests keyed by requirement_id
        req_ids = [r["id"] for r in requirements]
        tests: list[dict] = []
        if req_ids:
            placeholders = ",".join("?" * len(req_ids))
            tests = rows_to_dicts(conn.execute(
                f"""SELECT id, requirement_id, title, detailed_desc, version, base_version
                    FROM product_feature_tests WHERE requirement_id IN ({placeholders})
                    ORDER BY title""",
                req_ids,
            ).fetchall())

        # Assemble tree
        tests_by_req: dict[str, list] = {}
        for t in tests:
            tests_by_req.setdefault(t["requirement_id"], []).append(t)

        reqs_by_feat: dict[str, list] = {}
        for r in requirements:
            r["tests"] = tests_by_req.get(r["id"], [])
            # Staleness: parent feature version > requirement base_version
            feat = next((f for f in features if f["id"] == r["feature_id"]), None)
            r["stale"] = bool(feat and feat["version"] > r["base_version"])
            # Mark test staleness
            for t in r["tests"]:
                t["stale"] = r["version"] > t["base_version"]
            reqs_by_feat.setdefault(r["feature_id"], []).append(r)

        feats_by_epic: dict[str, list] = {}
        for f in features:
            f["requirements"] = reqs_by_feat.get(f["id"], [])
            epic = next((e for e in epics if e["id"] == f.get("epic_id")), None)
            f["stale"] = bool(epic and epic["version"] > f["base_version"])
            feats_by_epic.setdefault(f.get("epic_id", ""), []).append(f)

        for e in epics:
            e["features"] = feats_by_epic.get(e["id"], [])

        # Features not linked to any epic
        orphan_features = feats_by_epic.get("", []) + feats_by_epic.get(None, [])

        return {"epics": epics, "orphan_features": orphan_features}


# ---------------------------------------------------------------------------
# GET /api/{product_code}/iterators
# ---------------------------------------------------------------------------

@app.get("/api/{product_code}/iterators")
def list_iterators(product_code: str):
    with get_db() as conn:
        pid_row = conn.execute(
            "SELECT id FROM our_products WHERE code = ?", (product_code,)
        ).fetchone()
        if not pid_row:
            raise HTTPException(404, f"Product '{product_code}' not found")
        pid = pid_row["id"]

        iterators = rows_to_dicts(conn.execute(
            "SELECT id, name, description FROM iterators WHERE product_id = ? ORDER BY name",
            (pid,),
        ).fetchall())

        for it in iterators:
            vals = conn.execute(
                "SELECT value, position FROM iterator_values WHERE iterator_id = ? ORDER BY position",
                (it["id"],),
            ).fetchall()
            it["values"] = [{"value": v["value"], "position": v["position"]} for v in vals]

        return iterators


# ---------------------------------------------------------------------------
# PATCH /api/epics/{uuid}
# ---------------------------------------------------------------------------

@app.patch("/api/epics/{uuid}")
async def update_epic(uuid: str, request: Request):
    body = await request.json()
    allowed = {"name", "description", "rationale"}
    updates = {k: v for k, v in body.items() if k in allowed}
    if not updates:
        raise HTTPException(400, "No valid fields to update")

    with get_db() as conn:
        row = row_to_dict(conn.execute("SELECT * FROM epics WHERE id = ?", (uuid,)).fetchone())
        if not row:
            raise HTTPException(404, "Epic not found")

        new_version = row["version"] + 1

        # Snapshot current state to version history
        conn.execute(
            """INSERT INTO epic_versions (epic_id, version, name, description, rationale)
               VALUES (?, ?, ?, ?, ?)""",
            (uuid, row["version"], row["name"], row["description"], row["rationale"]),
        )

        # Build update
        fields = {**updates, "version": new_version, "updated_at": _now()}
        set_clause = ", ".join(f"{k} = ?" for k in fields)
        conn.execute(
            f"UPDATE epics SET {set_clause} WHERE id = ?",
            (*fields.values(), uuid),
        )
        conn.commit()

        updated = row_to_dict(conn.execute("SELECT * FROM epics WHERE id = ?", (uuid,)).fetchone())
        return updated


# ---------------------------------------------------------------------------
# PATCH /api/features/{uuid}
# ---------------------------------------------------------------------------

@app.patch("/api/features/{uuid}")
async def update_feature(uuid: str, request: Request):
    body = await request.json()
    allowed = {"short_desc", "detailed_desc", "target_release", "status", "source"}
    updates = {k: v for k, v in body.items() if k in allowed}
    if not updates:
        raise HTTPException(400, "No valid fields to update")

    with get_db() as conn:
        row = row_to_dict(
            conn.execute("SELECT * FROM product_features WHERE id = ?", (uuid,)).fetchone()
        )
        if not row:
            raise HTTPException(404, "Feature not found")

        new_version = row["version"] + 1

        # Snapshot to version history
        conn.execute(
            """INSERT INTO product_feature_versions
               (feature_id, version, short_desc, detailed_desc, target_release)
               VALUES (?, ?, ?, ?, ?)""",
            (uuid, row["version"], row["short_desc"], row["detailed_desc"],
             row.get("target_release")),
        )

        # Get current epic version for base_version
        epic_version = 1
        if row.get("epic_id"):
            ev = conn.execute(
                "SELECT version FROM epics WHERE id = ?", (row["epic_id"],)
            ).fetchone()
            if ev:
                epic_version = ev["version"]

        fields = {**updates, "version": new_version, "base_version": epic_version,
                  "updated_at": _now()}
        set_clause = ", ".join(f"{k} = ?" for k in fields)
        conn.execute(
            f"UPDATE product_features SET {set_clause} WHERE id = ?",
            (*fields.values(), uuid),
        )
        conn.commit()

        updated = row_to_dict(
            conn.execute("SELECT * FROM product_features WHERE id = ?", (uuid,)).fetchone()
        )
        return updated


# ---------------------------------------------------------------------------
# PATCH /api/requirements/{uuid}
# ---------------------------------------------------------------------------

@app.patch("/api/requirements/{uuid}")
async def update_requirement(uuid: str, request: Request):
    body = await request.json()
    allowed = {"title", "description", "acceptance_criteria", "status", "source"}
    updates = {k: v for k, v in body.items() if k in allowed}
    if not updates:
        raise HTTPException(400, "No valid fields to update")

    with get_db() as conn:
        row = row_to_dict(
            conn.execute("SELECT * FROM requirements WHERE id = ?", (uuid,)).fetchone()
        )
        if not row:
            raise HTTPException(404, "Requirement not found")

        new_version = row["version"] + 1

        # Snapshot
        conn.execute(
            """INSERT INTO requirement_versions
               (requirement_id, version, title, description, acceptance_criteria)
               VALUES (?, ?, ?, ?, ?)""",
            (uuid, row["version"], row["title"], row["description"],
             row.get("acceptance_criteria")),
        )

        # Get current feature version for base_version
        fv = conn.execute(
            "SELECT version FROM product_features WHERE id = ?", (row["feature_id"],)
        ).fetchone()
        feature_version = fv["version"] if fv else 1

        fields = {**updates, "version": new_version, "base_version": feature_version,
                  "updated_at": _now()}
        set_clause = ", ".join(f"{k} = ?" for k in fields)
        conn.execute(
            f"UPDATE requirements SET {set_clause} WHERE id = ?",
            (*fields.values(), uuid),
        )
        conn.commit()

        updated = row_to_dict(
            conn.execute("SELECT * FROM requirements WHERE id = ?", (uuid,)).fetchone()
        )
        return updated


# ---------------------------------------------------------------------------
# PATCH /api/tests/{uuid}
# ---------------------------------------------------------------------------

@app.patch("/api/tests/{uuid}")
async def update_test(uuid: str, request: Request):
    body = await request.json()
    allowed = {"title", "detailed_desc"}
    updates = {k: v for k, v in body.items() if k in allowed}
    if not updates:
        raise HTTPException(400, "No valid fields to update")

    with get_db() as conn:
        row = row_to_dict(
            conn.execute("SELECT * FROM product_feature_tests WHERE id = ?", (uuid,)).fetchone()
        )
        if not row:
            raise HTTPException(404, "Test not found")

        new_version = row["version"] + 1

        # Snapshot
        conn.execute(
            """INSERT INTO product_feature_test_versions
               (test_id, version, title, detailed_desc)
               VALUES (?, ?, ?, ?)""",
            (uuid, row["version"], row["title"], row["detailed_desc"]),
        )

        # Get current requirement version for base_version
        rv = conn.execute(
            "SELECT version FROM requirements WHERE id = ?", (row["requirement_id"],)
        ).fetchone()
        req_version = rv["version"] if rv else 1

        fields = {**updates, "version": new_version, "base_version": req_version,
                  "updated_at": _now()}
        set_clause = ", ".join(f"{k} = ?" for k in fields)
        conn.execute(
            f"UPDATE product_feature_tests SET {set_clause} WHERE id = ?",
            (*fields.values(), uuid),
        )
        conn.commit()

        updated = row_to_dict(
            conn.execute("SELECT * FROM product_feature_tests WHERE id = ?", (uuid,)).fetchone()
        )
        return updated


# ---------------------------------------------------------------------------
# Approve / Disapprove endpoints
# ---------------------------------------------------------------------------

def _set_approved(table: str, uuid: str, value: int) -> dict:
    with get_db() as conn:
        row = conn.execute(f"SELECT id FROM {table} WHERE id = ?", (uuid,)).fetchone()
        if not row:
            raise HTTPException(404, f"Entity not found in {table}")
        conn.execute(
            f"UPDATE {table} SET human_approved = ?, updated_at = ? WHERE id = ?",
            (value, _now(), uuid),
        )
        conn.commit()
        return {"id": uuid, "human_approved": value}


@app.post("/api/epics/{uuid}/approve")
def approve_epic(uuid: str):
    return _set_approved("epics", uuid, 1)


@app.post("/api/epics/{uuid}/disapprove")
def disapprove_epic(uuid: str):
    return _set_approved("epics", uuid, 0)


@app.post("/api/features/{uuid}/approve")
def approve_feature(uuid: str):
    return _set_approved("product_features", uuid, 1)


@app.post("/api/features/{uuid}/disapprove")
def disapprove_feature(uuid: str):
    return _set_approved("product_features", uuid, 0)


@app.post("/api/requirements/{uuid}/approve")
def approve_requirement(uuid: str):
    return _set_approved("requirements", uuid, 1)


@app.post("/api/requirements/{uuid}/disapprove")
def disapprove_requirement(uuid: str):
    return _set_approved("requirements", uuid, 0)


# ---------------------------------------------------------------------------
# Bulk approve
# ---------------------------------------------------------------------------

@app.post("/api/epics/{uuid}/bulk-approve")
def bulk_approve_epic(uuid: str):
    now = _now()
    with get_db() as conn:
        epic = conn.execute("SELECT id FROM epics WHERE id = ?", (uuid,)).fetchone()
        if not epic:
            raise HTTPException(404, "Epic not found")

        conn.execute(
            "UPDATE epics SET human_approved = 1, updated_at = ? WHERE id = ?",
            (now, uuid),
        )

        # All features under this epic
        features = conn.execute(
            "SELECT id FROM product_features WHERE epic_id = ?", (uuid,)
        ).fetchall()
        feat_ids = [f["id"] for f in features]

        if feat_ids:
            ph = ",".join("?" * len(feat_ids))
            conn.execute(
                f"UPDATE product_features SET human_approved = 1, updated_at = ? WHERE id IN ({ph})",
                [now, *feat_ids],
            )

            # All requirements under those features
            reqs = conn.execute(
                f"SELECT id FROM requirements WHERE feature_id IN ({ph})",
                feat_ids,
            ).fetchall()
            req_ids = [r["id"] for r in reqs]

            if req_ids:
                rph = ",".join("?" * len(req_ids))
                conn.execute(
                    f"UPDATE requirements SET human_approved = 1, updated_at = ? WHERE id IN ({rph})",
                    [now, *req_ids],
                )

                # Tests don't have human_approved, but update base_version awareness
                # Actually tests don't have human_approved column per schema, skip.

        conn.commit()
        count = 1 + len(feat_ids) + len(req_ids if feat_ids else [])
        return {"approved": count}


@app.post("/api/features/{uuid}/bulk-approve")
def bulk_approve_feature(uuid: str):
    now = _now()
    with get_db() as conn:
        feat = conn.execute(
            "SELECT id FROM product_features WHERE id = ?", (uuid,)
        ).fetchone()
        if not feat:
            raise HTTPException(404, "Feature not found")

        conn.execute(
            "UPDATE product_features SET human_approved = 1, updated_at = ? WHERE id = ?",
            (now, uuid),
        )

        reqs = conn.execute(
            "SELECT id FROM requirements WHERE feature_id = ?", (uuid,)
        ).fetchall()
        req_ids = [r["id"] for r in reqs]

        if req_ids:
            ph = ",".join("?" * len(req_ids))
            conn.execute(
                f"UPDATE requirements SET human_approved = 1, updated_at = ? WHERE id IN ({ph})",
                [now, *req_ids],
            )

        conn.commit()
        count = 1 + len(req_ids)
        return {"approved": count}


# ---------------------------------------------------------------------------
# Iterator value management
# ---------------------------------------------------------------------------

@app.post("/api/iterators/{uuid}/values")
async def add_iterator_value(uuid: str, request: Request):
    body = await request.json()
    value = body.get("value")
    if not value:
        raise HTTPException(400, "Missing 'value' field")

    with get_db() as conn:
        it = conn.execute("SELECT id FROM iterators WHERE id = ?", (uuid,)).fetchone()
        if not it:
            raise HTTPException(404, "Iterator not found")

        # Get next position
        max_pos = conn.execute(
            "SELECT COALESCE(MAX(position), 0) AS mp FROM iterator_values WHERE iterator_id = ?",
            (uuid,),
        ).fetchone()["mp"]

        try:
            conn.execute(
                "INSERT INTO iterator_values (iterator_id, value, position) VALUES (?, ?, ?)",
                (uuid, value, max_pos + 1),
            )
            conn.commit()
        except sqlite3.IntegrityError:
            raise HTTPException(409, f"Value '{value}' already exists on this iterator")

        return {"iterator_id": uuid, "value": value, "position": max_pos + 1}


@app.delete("/api/iterators/{uuid}/values/{value}")
def remove_iterator_value(uuid: str, value: str):
    with get_db() as conn:
        deleted = conn.execute(
            "DELETE FROM iterator_values WHERE iterator_id = ? AND value = ?",
            (uuid, value),
        ).rowcount
        conn.commit()
        if not deleted:
            raise HTTPException(404, "Value not found on this iterator")
        return {"deleted": True}


# ---------------------------------------------------------------------------
# POST /api/{product_code}/feedback
# ---------------------------------------------------------------------------

@app.post("/api/{product_code}/feedback")
async def submit_feedback(product_code: str, request: Request):
    body = await request.json()
    entity_type = body.get("entity_type")
    entity_id = body.get("entity_id")
    feedback_text = body.get("feedback_text")

    if not all([entity_type, entity_id, feedback_text]):
        raise HTTPException(400, "Missing required fields: entity_type, entity_id, feedback_text")

    with get_db() as conn:
        pid_row = conn.execute(
            "SELECT id FROM our_products WHERE code = ?", (product_code,)
        ).fetchone()
        if not pid_row:
            raise HTTPException(404, f"Product '{product_code}' not found")

        _ensure_feedback_table(conn)

        conn.execute(
            """INSERT INTO feedback (product_id, entity_type, entity_id, feedback_text)
               VALUES (?, ?, ?, ?)""",
            (pid_row["id"], entity_type, entity_id, feedback_text),
        )
        conn.commit()
        return {"stored": True}


# ---------------------------------------------------------------------------
# Static files (must be last — catches all unmatched routes)
# ---------------------------------------------------------------------------

app.mount("/", StaticFiles(directory=str(STATIC_DIR), html=True), name="static")


# ---------------------------------------------------------------------------
# CLI entry point
# ---------------------------------------------------------------------------

def main():
    global DB_PATH

    parser = argparse.ArgumentParser(description="PM Review Webtool")
    parser.add_argument(
        "--project", required=True,
        help="Path to the project root (database at {project}/.claude/db/marketing.sqlite)",
    )
    parser.add_argument("--port", type=int, default=8420, help="Port to serve on (default: 8420)")
    parser.add_argument("--host", default="127.0.0.1", help="Host to bind to (default: 127.0.0.1)")
    parser.add_argument("--no-browser", action="store_true", help="Don't auto-open browser")
    args = parser.parse_args()

    project = Path(args.project).expanduser().resolve()
    DB_PATH = project / ".claude" / "db" / "marketing.sqlite"

    if not DB_PATH.exists():
        print(f"Error: Database not found at {DB_PATH}")
        raise SystemExit(1)

    if not STATIC_DIR.exists():
        print(f"Warning: Static directory not found at {STATIC_DIR}, API-only mode")

    print(f"Database: {DB_PATH}")
    print(f"Static:   {STATIC_DIR}")
    print(f"Server:   http://{args.host}:{args.port}")

    if not args.no_browser:
        # Open browser after a short delay to let server start
        import threading
        threading.Timer(1.0, webbrowser.open, args=[f"http://{args.host}:{args.port}"]).start()

    uvicorn.run(app, host=args.host, port=args.port, log_level="info")


if __name__ == "__main__":
    main()
