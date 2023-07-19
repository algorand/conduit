from e2e_common.indexer_db import IndexerDB
from e2e_conduit.fixtures import importers, exporters
from e2e_conduit.scenarios import Scenario


class FollowerIndexerScenario(Scenario):
    def __init__(self, name, importer, processors, exporter):
        super().__init__(
            name=name,
            importer=importer,
            processors=processors,
            exporter=exporter,
        )

    def validate(self) -> list[str]:
        """
        Validate the scenario.
        A non empty list of error messages signals a failed validation.
        """
        init_args = {
            item.split("=")[0]: item.split("=")[1]
            for item in self.exporter.config_input["connection-string"].split()
        }
        idb = IndexerDB(
            host=init_args["host"],
            port=init_args["port"],
            user=init_args["user"],
            password=init_args["password"],
            dbname=init_args["dbname"],
        )
        errors = []
        try:
            indexer_first_round, last_txn_round = idb.get_txn_min_max_round()
        except Exception as e:
            errors.append(f"Failed to get min/max round from indexer: {e}")

        try:
            indexer_final_round = idb.get_block_header_final_round()
            if indexer_final_round != self.importer.lastblock:
                errors.append(
                    f"Indexer final round {indexer_final_round} does not match importer last block {self.importer.lastblock}"
                )
        except Exception as e:
            errors.append(f"Failed to get final round from indexer: {e}")

        return errors


def follower_indexer_scenario(sourcenet):
    return FollowerIndexerScenario(
        "follower_indexer_scenario",
        importer=importers.FollowerAlgodImporter(sourcenet),
        processors=[],
        exporter=exporters.PostgresqlExporter(),
    )
