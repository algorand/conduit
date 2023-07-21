import time

from e2e_common.indexer_db import IndexerDB
from e2e_conduit.fixtures import importers, exporters
from e2e_conduit.scenarios import Scenario


class _Errors:
    # SQL errors:
    def block_header_final_round_query_error(self, e):
        return f"Failed to get final round from indexer block_header table: {e}"

    def txn_min_max_query_error(self, e):
        return f"Failed to get min/max round from indexer txn table: {e}"

    def table_row_count_query_error(self, table_name, e):
        return f"Failed to get row count from indexer {table_name} table: {e}"

    # Logic errors:
    def txn_table_biggest_round_too_big(self, last_txn_round):
        return f"Indexer table txn has round={last_txn_round} greater than network's final round={self.importer.lastblock}"

    def block_header_round_mismatch(self, indexer_final_round):
        return f"Indexer table block_header has final round={indexer_final_round} different from network's final round={self.importer.lastblock}"

    def delete_task_txn_rounds_different(self, first_txn_round, last_txn_round):
        return f"""Indexer table txn has smallest round={first_txn_round} different from greatest round={last_txn_round}.
This is problematic for the delete task because {self.exporter.config_input["delete_task"]=} so we should only keep transactions for the very last round."""


class FollowerIndexerScenario(Scenario, _Errors):
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
                errors.append(self.txn_table_biggest_round_too_big(last_txn_round))
        except Exception as e:
            errors.append(self.txn_min_max_query_error(e))

        try:
            indexer_final_round = idb.get_block_header_final_round()
            if indexer_final_round != self.importer.lastblock:
                errors.append(self.block_header_round_mismatch(indexer_final_round))
        except Exception as e:
            errors.append(self.block_header_final_round_query_error(e))

        return errors


class FollowerIndexerScenarioWithDeleteTask(Scenario, _Errors):
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
                            self.delete_task_txn_rounds_different(
                                first_txn_round, last_txn_round
                            )
                        )

                    if last_txn_round > self.importer.lastblock:
                        errors.append(
                            self.txn_table_biggest_round_too_big(last_txn_round)
                        )

                except Exception as e:
                    errors.append(self.txn_min_max_query_error(e))
        except Exception as e:
            errors.append(self.table_row_count_query_error("txn", e))

        return errors
