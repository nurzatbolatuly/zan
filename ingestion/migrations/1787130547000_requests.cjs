/** @type {import('node-pg-migrate').ColumnDefinitions | undefined} */
exports.shorthands = undefined;

exports.up = (pgm) => {
  pgm.sql(`
    CREATE TYPE request_status AS ENUM ('pending', 'processing', 'completed', 'failed');
    CREATE TYPE request_step_status AS ENUM ('pending', 'running', 'success', 'failed');

    CREATE TABLE requests (
      id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      status request_status NOT NULL DEFAULT 'pending',
      query_text TEXT NOT NULL,
      include_document BOOLEAN NOT NULL DEFAULT false,
      document_type TEXT,
      result_summary TEXT,
      error_message TEXT,
      created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
      updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
    );

    CREATE TABLE request_steps (
      id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      request_id UUID NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
      agent_name TEXT NOT NULL,
      ordinal INTEGER NOT NULL,
      status request_step_status NOT NULL DEFAULT 'pending',
      input JSONB,
      output JSONB,
      error_message TEXT,
      started_at TIMESTAMPTZ,
      finished_at TIMESTAMPTZ,
      created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
      UNIQUE (request_id, ordinal)
    );

    CREATE TABLE documents (
      id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      request_id UUID NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
      document_type TEXT NOT NULL,
      file_format TEXT NOT NULL,
      content TEXT NOT NULL,
      created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    );

    CREATE INDEX request_steps_request_id_idx ON request_steps (request_id);
    CREATE INDEX documents_request_id_idx ON documents (request_id);
  `);
};

exports.down = (pgm) => {
  pgm.sql(`
    DROP TABLE IF EXISTS documents;
    DROP TABLE IF EXISTS request_steps;
    DROP TABLE IF EXISTS requests;
    DROP TYPE IF EXISTS request_step_status;
    DROP TYPE IF EXISTS request_status;
  `);
};
