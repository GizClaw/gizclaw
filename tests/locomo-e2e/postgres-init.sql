-- Disposable LoCoMo application identity; administrative setup stays separate.
CREATE ROLE gizclaw_test LOGIN PASSWORD 'gizclaw_test';
ALTER DATABASE mem0_locomo OWNER TO gizclaw_test;
