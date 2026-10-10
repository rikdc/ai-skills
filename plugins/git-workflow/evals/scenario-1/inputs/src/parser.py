"""CSV record parser for the Acme data pipeline."""

import csv
import io
from typing import Iterator


def parse_records(raw: str) -> Iterator[dict]:
    """Yield parsed dicts from a CSV string.

    Args:
        raw: Raw CSV text (may include a header row).

    Yields:
        One dict per data row with stripped field values.
    """
    reader = csv.DictReader(io.StringIO(raw))
    for row in reader:
        # Strip whitespace from every value to avoid downstream mismatches.
        # Previously values were yielded as-is, causing lookup failures when
        # the source data contained trailing spaces.
        yield {k: v.strip() for k, v in row.items()}
