CREATE TABLE IF NOT EXISTS solution_streams (
    ts TIMESTAMP TIME INDEX,
    environment_id STRING,
    feature TEXT,
    stage STRING,
    value DOUBLE,
    attributes MAP(STRING, STRING),
    PRIMARY KEY (ts, environment_id, feature)
);
