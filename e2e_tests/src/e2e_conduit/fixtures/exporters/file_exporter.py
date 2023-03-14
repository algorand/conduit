import logging
import random
import string
import sys
import time

from e2e_conduit.fixtures.plugin_fixture import PluginFixture
from e2e_common.util import atexitrun, xrun

logger = logging.getLogger(__name__)


class FileExporter(PluginFixture):
    def __init__(self):
        super().__init__()

    @property
    def name(self):
        return "filewriter"

    def setup(self, accumulated_config):
        try:
            conduit_dir = accumulated_config["conduit_dir"]
        except KeyError as exc:
            logger.error(
                f"FileExporter needs to be provided with the proper config: {exc}"
            )
            raise
        
            atexitrun(["docker", "kill", self.container_name])

    def resolve_config_input(self):
        self.config_input = {
            "connection-string": f"host=localhost port={self.port} user={self.user} password={self.password} dbname={self.db_name} sslmode=disable",
            "max-conn": self.max_conn,
        }
