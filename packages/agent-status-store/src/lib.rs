//! SQLite schema and domain queries shared by the ingester, server, and offline debugger.
//! Callers own native `rusqlite::Connection` and `Transaction` values.
#![doc = include_str!("../README.md")]

pub mod read;
pub mod schema;
pub mod write;

use rusqlite::{Connection, OpenFlags};
use std::{path::Path, time::Duration};

pub fn open_writer(path: impl AsRef<Path>) -> Result<Connection> {
    let path = path.as_ref();
    if path != Path::new(":memory:")
        && let Some(parent) = path.parent()
        && !parent.as_os_str().is_empty()
    {
        std::fs::create_dir_all(parent)?;
    }
    let mut db = Connection::open_with_flags(
        path,
        OpenFlags::SQLITE_OPEN_READ_WRITE
            | OpenFlags::SQLITE_OPEN_CREATE
            | OpenFlags::SQLITE_OPEN_NO_MUTEX,
    )?;
    db.busy_timeout(Duration::from_secs(5))?;
    db.execute_batch(
        "PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA foreign_keys=ON;",
    )?;
    schema::migrate(&mut db)?;
    if db.pragma_query_value(None, "auto_vacuum", |row| row.get::<_, i64>(0))? != 2 {
        // Existing files need one rewrite. The temporary copy and WAL can need
        // up to twice the database size in free disk space.
        db.execute_batch(
            "PRAGMA auto_vacuum=INCREMENTAL; VACUUM; PRAGMA wal_checkpoint(TRUNCATE);",
        )
        .map_err(Error::AutoVacuum)?;
    }
    Ok(db)
}

pub fn open_reader(path: impl AsRef<Path>) -> Result<Connection> {
    let db = Connection::open_with_flags(
        path,
        OpenFlags::SQLITE_OPEN_READ_ONLY | OpenFlags::SQLITE_OPEN_NO_MUTEX,
    )?;
    db.busy_timeout(Duration::from_secs(5))?;
    db.pragma_update(None, "foreign_keys", true)?;
    Ok(db)
}

#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("SQLite operation failed")]
    Sql(#[from] rusqlite::Error),
    #[error("enabling incremental auto-vacuum failed")]
    AutoVacuum(#[source] rusqlite::Error),
    #[error("stored telemetry JSON is invalid")]
    Json(#[from] serde_json::Error),
    #[error("database filesystem operation failed")]
    Io(#[from] std::io::Error),
    #[error("agent not found")]
    NotFound,
    #[error("unsupported database schema version {0}")]
    SchemaVersion(i64),
    #[error("agents harness constraint does not match the version 10 schema")]
    AgentSchema,
    #[error("foreign key check failed for table {table}, parent {parent}, row {rowid:?}")]
    ForeignKey {
        table: String,
        parent: String,
        rowid: Option<i64>,
    },
}

pub type Result<T> = std::result::Result<T, Error>;
