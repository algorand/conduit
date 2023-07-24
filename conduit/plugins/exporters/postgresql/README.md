# PostgreSQL Exporter

Write block data to a postgres database.

The database maintained by this plugin is designed to serve the Indexer API.

## Connection string

The connection string is defined by the [pgx](https://github.com/jackc/pgconn) database driver.

For most deployments, you can use the following format:

```psql
host={url} port={port} user={user} password={password} dbname={db_name} sslmode={enable|disable}
```

For additional details, refer to the [parsing documentation here](https://pkg.go.dev/github.com/jackc/pgx/v4/pgxpool@v4.11.0#ParseConfig).

## Data Pruning

The delete-task prunes old transactions according to its configuration. This can be used to limit the size of the database.

## Configuration

```yml @sample.yaml
name: postgresql
config:
    # Pgsql connection string
    # See https://github.com/jackc/pgconn for more details
    connection-string: "host= port=5432 user= password= dbname="
  
    # Maximum connection number for connection pool
    # This means the total number of active queries that can be running
    # concurrently can never be more than this
    max-conn: 20
  
    # The delete task prunes old transactions according to its configuration.
    # By default transactions are not deleted.
    delete-task:
        # Interval used to prune the data. The values can be -1 to run at startup,
        # 0 to disable, or N to run every N rounds.
        interval: 0
    
        # Rounds to keep
        rounds: 100000
```

## TODO - Mermaid

```mermaid
graph TD
    roundChan((roundChan))
    importedBlocksChan((importedBlocksChan))
    processorChans0("processorChans[0]")
    processorChans1("processorChans[1]")
    exporterChan((exporterChan))

    roundChan -->|inChan| ImportHandler1[ImportHandler 1]
    roundChan -->|inChan| ImportHandler2[ImportHandler 2]
    roundChan -->|inChan| ImportHandler3[ImportHandler 3]
    ImportHandler1 -->|result| importedBlocksChan
    ImportHandler2 -->|result| importedBlocksChan
    ImportHandler3 -->|result| importedBlocksChan

    importedBlocksChan --> OrderResults[Order Results]
    OrderResults --> processorChans0

    processorChans0 --> ProcessHandler1[ProcessHandler 1]
    ProcessHandler1 --> processorChans1

    processorChans1 --> ProcessHandler2[ProcessHandler 2]
    ProcessHandler2 --> exporterChan

    exporterChan --> ExportHandler[ExportHandler]
```

### V2

```mermaid
graph TD
    style roundChan fill:#f9f,stroke:#333,stroke-width:2px
    style importedBlocksChan fill:#f9f,stroke:#333,stroke-width:2px
    style processorChans0 fill:#f9f,stroke:#333,stroke-width:2px
    style processorChans1 fill:#f9f,stroke:#333,stroke-width:2px
    style exporterChan fill:#f9f,stroke:#333,stroke-width:2px

    roundChan[[roundChan]]
    importedBlocksChan[[importedBlocksChan]]
    processorChans0("processorChans[0]")
    processorChans1("processorChans[1]")
    exporterChan[[exporterChan]]

    roundChan -->|inChan| BlockGetter1((BlockGetter 1))
    roundChan -->|inChan| BlockGetter2((BlockGetter 2))
    roundChan -->|inChan| BlockGetter3((BlockGetter 3))
    BlockGetter1 -->|result| importedBlocksChan
    BlockGetter2 -->|result| importedBlocksChan
    BlockGetter3 -->|result| importedBlocksChan

    importedBlocksChan --> OrderResults((Order Results))
    OrderResults --> processorChans0

    processorChans0 --> ProcessHandler1((ProcessHandler 1))
    ProcessHandler1 --> processorChans1

    processorChans1 --> ProcessHandler2((ProcessHandler 2))
    ProcessHandler2 --> exporterChan

    exporterChan --> ExportHandler((ExportHandler))
```

### V3

```mermaid
graph LR
    style roundChan fill:#f9f,stroke:#333,stroke-width:2px
    style importedBlocksChan fill:#f9f,stroke:#333,stroke-width:2px
    style processorChans0 fill:#f9f,stroke:#333,stroke-width:2px
    style exporterChan fill:#f9f,stroke:#333,stroke-width:2px

    roundChan[[roundChan]]
    processorChans0("processorChans[0]")
    exporterChan[[exporterChan]]

    roundChan -->|inChan| BlockGetter1((BlockGetter 1))
    roundChan -->|inChan| BlockGetter2((BlockGetter 2))
    roundChan -->|inChan| BlockGetter3((BlockGetter 3))

    subgraph ImportHandler
        importedBlocksChan[[importedBlocksChan]]
        
        BlockGetter1 -->|result| importedBlocksChan
        BlockGetter2 -->|result| importedBlocksChan
        BlockGetter3 -->|result| importedBlocksChan

        importedBlocksChan --> OrderResults((Order Results))
    end

    OrderResults --> processorChans0

    processorChans0 --> ProcessHandler1((ProcessHandler 1))
    ProcessHandler1 --> exporterChan

    exporterChan --> ExportHandler((ExportHandler))
```
