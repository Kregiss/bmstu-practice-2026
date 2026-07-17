CREATE TABLE people
(
    id UInt64,
    last_name String,
    first_name String,
    middle_name String,
    full_name String
)
ENGINE = MergeTree()
ORDER BY id;

INSERT INTO people
SELECT
    id,
    last_name,
    first_name,
    middle_name,
    concat(last_name, ' ', first_name, ' ', middle_name) as full_name
FROM file('people.csv', 'CSV', 'id UInt32, last_name String, first_name String, middle_name String');

ALTER TABLE people
ADD INDEX idx_full_name_text full_name
TYPE text(tokenizer = 'splitByNonAlpha', preprocessor = lower(full_name))
GRANULARITY 1;

ALTER TABLE people MATERIALIZE INDEX idx_full_name_text;