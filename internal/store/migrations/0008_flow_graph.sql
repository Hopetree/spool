-- +goose Up
CREATE TABLE flow_nodes (
  id TEXT PRIMARY KEY,
  flow_id TEXT NOT NULL REFERENCES flows(id) ON DELETE CASCADE,
  node_key TEXT NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('start','tool')),
  name TEXT NOT NULL DEFAULT '',
  tool_id TEXT NOT NULL DEFAULT '',
  mapping_json TEXT NOT NULL DEFAULT '{}',
  timeout_sec INTEGER NOT NULL DEFAULT 0,
  retry_count INTEGER NOT NULL DEFAULT 0,
  retry_delay_sec INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1,
  x REAL NOT NULL DEFAULT 0,
  y REAL NOT NULL DEFAULT 0,
  UNIQUE(flow_id, node_key)
);
CREATE INDEX idx_flow_nodes_flow ON flow_nodes(flow_id);

CREATE TABLE flow_edges (
  id TEXT PRIMARY KEY,
  flow_id TEXT NOT NULL REFERENCES flows(id) ON DELETE CASCADE,
  from_key TEXT NOT NULL,
  to_key TEXT NOT NULL,
  condition TEXT NOT NULL DEFAULT 'success',
  condition_value TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_flow_edges_flow ON flow_edges(flow_id);

ALTER TABLE step_runs ADD COLUMN node_id TEXT NOT NULL DEFAULT '';

-- Migrate existing stage-based flows into a graph.
INSERT INTO flow_nodes (id, flow_id, node_key, type, name, x, y)
SELECT 'start-' || id, id, 'start', 'start', '开始', -260, 40 FROM flows;

INSERT INTO flow_nodes (id, flow_id, node_key, type, name, tool_id, mapping_json, timeout_sec, retry_count, retry_delay_sec, enabled, x, y)
SELECT st.id, stg.flow_id, st.id, 'tool', st.name, st.tool_id, st.param_mapping_json,
       st.timeout_sec, st.retry_count, st.retry_delay_sec, st.enabled,
       (stg.idx - 1) * 260, (st.idx - 1) * 160
FROM flow_steps st
JOIN flow_stages stg ON stg.id = st.stage_id;

INSERT INTO flow_edges (id, flow_id, from_key, to_key, condition)
SELECT lower(hex(randomblob(16))), stg.flow_id, 'start', st.id, 'always'
FROM flow_steps st
JOIN flow_stages stg ON stg.id = st.stage_id
WHERE stg.idx = 1;

INSERT INTO flow_edges (id, flow_id, from_key, to_key, condition)
SELECT lower(hex(randomblob(16))), stg1.flow_id, st1.id, st2.id,
       CASE WHEN stg2.always_run = 1 OR f.failure_policy = 'continue' THEN 'always' ELSE 'success' END
FROM flow_steps st1
JOIN flow_stages stg1 ON stg1.id = st1.stage_id
JOIN flows f ON f.id = stg1.flow_id
JOIN flow_stages stg2 ON stg2.flow_id = stg1.flow_id AND stg2.idx = stg1.idx + 1
JOIN flow_steps st2 ON st2.stage_id = stg2.id;

-- +goose Down
ALTER TABLE step_runs DROP COLUMN node_id;
DROP TABLE flow_edges;
DROP TABLE flow_nodes;
