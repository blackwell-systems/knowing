ALTER TABLE edges ADD COLUMN observation_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE edges ADD COLUMN last_observed INTEGER NOT NULL DEFAULT 0;

CREATE TABLE route_symbols (
    service_name  TEXT NOT NULL,
    route_pattern TEXT NOT NULL,
    node_hash     BLOB NOT NULL,
    mapping_type  TEXT NOT NULL,
    created_at    INTEGER NOT NULL,
    PRIMARY KEY (service_name, route_pattern, mapping_type)
);

CREATE INDEX idx_route_symbols_node ON route_symbols(node_hash);
CREATE INDEX idx_edges_provenance ON edges(provenance);
CREATE INDEX idx_edges_last_observed ON edges(last_observed);
