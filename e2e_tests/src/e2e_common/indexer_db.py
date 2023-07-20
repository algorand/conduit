import psycopg2


class IndexerDB:
    def __init__(self, host, port, user, password, dbname):
        self.host = host
        self.port = port
        self.user = user
        self.password = password
        self.dbname = dbname

    @classmethod
    def from_connection_string(cls, connection_string):
        init_args = {
            keyval.split("=")[0]: keyval.split("=")[1]
            for keyval in connection_string.split()
        }
        return cls(
            host=init_args["host"],
            port=init_args["port"],
            user=init_args["user"],
            password=init_args["password"],
            dbname=init_args["dbname"],
        )

    def select_one(self, query) -> tuple:
        with psycopg2.connect(
            host=self.host,
            port=self.port,
            user=self.user,
            password=self.password,
            dbname=self.dbname,
        ) as connection:
            with connection.cursor() as cursor:
                cursor.execute(query)
                return cursor.fetchone()  # type: ignore

    def get_txn_min_max_round(self):
        min_round, max_round = self.select_one("SELECT min(round), max(round) FROM txn")
        return min_round, max_round

    def get_table_row_count(self, table_name):
        return self.select_one(f"SELECT count(*) FROM {table_name}")[0]

    def get_block_header_final_round(self):
        return self.select_one("SELECT max(round) FROM block_header")[0]
