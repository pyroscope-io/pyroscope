# Data loader

The tool populates a database with sample data.

Generate sample data with dataloader:
```
go run ./pkg/testing/load/cmd/dataloader -path config.yml
```

See example `config.yml` in `dataloader` directory for details. The loader writes data directly to the storage, the path should be specified in the configuration file:
```yaml
storage:
  path: test_storage
```

Start server with `-storge-path` option (if `pyroscope server` is run in a container, you can map the volume):
```
pyroscope server -storage-path test_storage
```
