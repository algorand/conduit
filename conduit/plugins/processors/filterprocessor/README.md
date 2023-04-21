# Filter Processor

## Configuration
```yml @sample.yaml
name: filter_processor
config:
  # Whether or not the expression searches inner transactions for matches.
  search-inner: true

  # Whether or not to include the entire transaction group when the filter
  # conditions are met.
  omit-group-transactions: true

  # The list of filter expressions to use when matching transactions.
  filters:
    - any:
        - tag: txn.rcv
          expression-type: exact
          expression: "ADDRESS"
```