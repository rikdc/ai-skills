# Acme Data Pipeline

A lightweight ETL tool for processing CSV exports from Acme's CRM.

## Overview

The pipeline ingests CSV files produced by the CRM, validates and transforms
each record, and loads the results into the data warehouse.

## Modules

- **`src/parser.py`** — Reads raw CSV input and yields cleaned record dicts.
- **`src/feature.py`** — Sends transactional emails (e.g., welcome messages).

## Getting Started

```bash
pip install -r requirements.txt
python -m acme.pipeline --input data/export.csv
```

## Configuration

Set the following environment variables before running:

| Variable         | Description                          | Default               |
|------------------|--------------------------------------|-----------------------|
| `SMTP_HOST`      | Hostname of the outbound SMTP server | `localhost`           |
| `SMTP_PORT`      | Port for SMTP                        | `25`                  |
| `FROM_ADDRESS`   | Sender address for system emails     | `noreply@acme.example.com` |

## Contributing

1. Fork the repository and create a feature branch.
2. Run `make checks` before opening a pull request.
3. Follow the project's commit message conventions.
