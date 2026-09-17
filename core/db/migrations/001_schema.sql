-- Core schema for the portfolio site.

CREATE TABLE IF NOT EXISTS admins (
    id            SERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS profile (
    id              SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    name            TEXT NOT NULL DEFAULT '',
    title           TEXT NOT NULL DEFAULT '',
    tagline         TEXT NOT NULL DEFAULT '',
    hero_subtitle   TEXT NOT NULL DEFAULT '',
    summary         TEXT NOT NULL DEFAULT '',
    location        TEXT NOT NULL DEFAULT '',
    email           TEXT NOT NULL DEFAULT '',
    linkedin_url    TEXT NOT NULL DEFAULT '',
    github_url      TEXT NOT NULL DEFAULT '',
    resume_url      TEXT NOT NULL DEFAULT '',
    photo_url       TEXT NOT NULL DEFAULT '',
    years_experience SMALLINT NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS skill_groups (
    id         SERIAL PRIMARY KEY,
    category   TEXT NOT NULL,
    level      TEXT NOT NULL DEFAULT '',
    items      TEXT NOT NULL DEFAULT '',
    sort_order INT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS experiences (
    id         SERIAL PRIMARY KEY,
    company    TEXT NOT NULL,
    role       TEXT NOT NULL,
    location   TEXT NOT NULL DEFAULT '',
    start_date TEXT NOT NULL DEFAULT '',
    end_date   TEXT NOT NULL DEFAULT '',
    bullets    TEXT NOT NULL DEFAULT '',
    sort_order INT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS education (
    id          SERIAL PRIMARY KEY,
    institution TEXT NOT NULL,
    degree      TEXT NOT NULL,
    field       TEXT NOT NULL DEFAULT '',
    start_year  INT,
    end_year    INT,
    details     TEXT NOT NULL DEFAULT '',
    sort_order  INT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS home_features (
    id          SERIAL PRIMARY KEY,
    title       TEXT NOT NULL,
    description TEXT NOT NULL,
    sort_order  INT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS courses (
    id           SERIAL PRIMARY KEY,
    slug         TEXT NOT NULL UNIQUE,
    title        TEXT NOT NULL,
    summary      TEXT NOT NULL DEFAULT '',
    description  TEXT NOT NULL DEFAULT '',
    level        TEXT NOT NULL DEFAULT '',
    duration     TEXT NOT NULL DEFAULT '',
    price_cents  INT NOT NULL DEFAULT 0,
    currency     TEXT NOT NULL DEFAULT 'USD',
    image_url    TEXT NOT NULL DEFAULT '',
    is_published BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order   INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_courses_published ON courses (is_published, sort_order);

CREATE TABLE IF NOT EXISTS availability_rules (
    id           SERIAL PRIMARY KEY,
    weekday      SMALLINT NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    start_time   TIME NOT NULL,
    end_time     TIME NOT NULL,
    slot_minutes INT NOT NULL DEFAULT 30,
    active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS bookings (
    id         SERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    email      TEXT NOT NULL,
    message    TEXT NOT NULL DEFAULT '',
    slot_start TIMESTAMPTZ NOT NULL,
    slot_end   TIMESTAMPTZ NOT NULL,
    status     TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (slot_start)
);

CREATE INDEX IF NOT EXISTS idx_bookings_slot_start ON bookings (slot_start);
