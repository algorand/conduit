import time

from e2e_common.indexer_db import IndexerDB
from e2e_conduit.fixtures import importers, exporters
from e2e_conduit.scenarios import Scenario


class FollowerIndexerScenario(Scenario):
    def __init__(self, sourcenet):
        super().__init__(
            name="follower_indexer_scenario",
            importer=importers.FollowerAlgodImporter(sourcenet),
            processors=[],
            exporter=exporters.PostgresqlExporter(),
        )

    def get_validation_errors(self) -> list[str]:
        """
        validation checks that indexer tables block_header and txn makes sense when
        compared with the importer lastblock which is the network's last round in the
        network's blocks table.
        """
        time.sleep(1)

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
            _, last_txn_round = idb.get_txn_min_max_round()
            if last_txn_round > self.importer.lastblock:
                errors.append(
                    f"Indexer last txn round {last_txn_round} is greater than importer last block {self.importer.lastblock}"
                )
        except Exception as e:
            errors.append(f"Failed to get min/max round from indexer txn table: {e}")

        try:
            indexer_final_round = idb.get_block_header_final_round()
            if indexer_final_round != self.importer.lastblock:
                errors.append(
                    f"Indexer final round {indexer_final_round} does not match importer last block {self.importer.lastblock}"
                )
        except Exception as e:
            errors.append(
                f"Failed to get final round from indexer block_header table: {e}"
            )

        return errors


class FollowerIndexerScenarioWithDeleteTask(Scenario):
    def __init__(self, sourcenet):
        super().__init__(
            name="follower_indexer_scenario_with_delete_task",
            importer=importers.FollowerAlgodImporter(sourcenet),
            processors=[],
            exporter=exporters.PostgresqlExporter(delete_interval=1, delete_rounds=1),
        )

    def get_validation_errors(self) -> list[str]:
        """
        validation checks that txn either contains no rows or that
        the max round contained is the same as the network's lastblock
        """

        # sleep for 3 seconds to allow delete_task iteration to wake up after 2 seconds
        # for its final pruning when Conduit is all caught up with the network
        time.sleep(3)

        idb = IndexerDB.from_connection_string(
            self.exporter.config_input["connection-string"]
        )

        errors = []
        try:
            num_txn_rows = idb.get_table_row_count("txn")

            if num_txn_rows > 0:
                try:
                    first_txn_round, last_txn_round = idb.get_txn_min_max_round()

                    if first_txn_round != last_txn_round:
                        errors.append(
                            f"Indexer first txn round {first_txn_round} is not equal to last txn round {last_txn_round}"
                        )

                    if last_txn_round != self.importer.lastblock:
                        errors.append(
                            f"Indexer last txn round {last_txn_round} is not equal to importer last block {self.importer.lastblock}"
                        )

                except Exception as e:
                    errors.append(
                        f"Failed to get min/max round from indexer txn table: {e}"
                    )
        except Exception as e:
            errors.append(f"Failed to get row count from indexer txn table: {e}")

        return errors
