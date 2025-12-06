# Test Dataset for Training Integration Tests

This directory contains a minimal test dataset for training integration tests.

## Structure

```
test-dataset-1/
├── metadata.json          # Dataset metadata
├── manifest.json          # Snapshot manifest (optional)
└── screenshots/           # Labeled snapshots
    ├── normal/
    │   ├── snapshot_001.jpg
    │   ├── snapshot_002.jpg
    │   └── ...
    └── anomaly/
        ├── snapshot_001.jpg
        ├── snapshot_002.jpg
        └── ...
```

## Dataset ID

The dataset ID used in tests is: `test-dataset-1`

## Labels

- `normal`: Normal/expected behavior snapshots
- `anomaly`: Anomalous/unexpected behavior snapshots

