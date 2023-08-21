import atexit
import os
import logging
import shutil
import subprocess
import sys
import tempfile
import time

import yaml

from e2e_common.util import find_binary
from e2e_conduit.subslurp import subslurp

logger = logging.getLogger(__name__)


class ConduitE2ETestRunner:
    def __init__(self, conduit_bin, keep_temps=False):
        self.conduit_bin = find_binary(conduit_bin, binary_name="conduit")
        self.keep_temps = keep_temps

    def setup_scenario(self, scenario):
        # Setup conduit_dir for conduit data dir
        scenario.conduit_dir = tempfile.mkdtemp()
        if not self.keep_temps:
            atexit.register(shutil.rmtree, scenario.conduit_dir, onerror=logger.error)
        else:
            logger.info(f"leaving temp dir {scenario.conduit_dir}")

        scenario.accumulated_config = {
            "conduit_dir": scenario.conduit_dir,
        }

        for plugin in [scenario.importer, *scenario.processors, scenario.exporter]:
            plugin.setup(scenario.accumulated_config)
            plugin.resolve_config()
            scenario.accumulated_config = {
                **scenario.accumulated_config,
                **plugin.config_output,
            }

        # Write conduit config to data directory
        with open(
            os.path.join(scenario.conduit_dir, "conduit.yml"), "w"
        ) as conduit_cfg:
            yaml.dump(
                {
                    "log-level": "info",
                    "retry-count": 1,
                    "importer": {
                        "name": scenario.importer.name,
                        "config": scenario.importer.config_input,
                    },
                    "processors": [
                        {
                            "name": processor.name,
                            "config": processor.config_input,
                        }
                        for processor in scenario.processors
                    ],
                    "exporter": {
                        "name": scenario.exporter.name,
                        "config": scenario.exporter.config_input,
                    },
                },
                conduit_cfg,
            )

    def run_scenario(self, scenario):
        # Run conduit
        start = time.time()
        cmd = [self.conduit_bin, "-d", scenario.conduit_dir]
        logger.info(f"running scenario {scenario.name}")
        logger.debug("%s", " ".join(map(repr, cmd)))
        indexerdp = subprocess.Popen(
            cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT
        )
        atexit.register(indexerdp.kill)
        indexerout = subslurp(indexerdp.stdout)

        logger.info(f"Waiting for conduit to reach round {scenario.importer.lastblock}")

        try:
            indexerout.run(scenario.importer.lastblock)
        except RuntimeError as exc:
            logger.error(f"{exc}")
            logger.error(
                f"conduit hit an error during execution: {indexerout.error_log}"
            )
            sys.stderr.write(indexerout.dump())
            return 1

        if indexerout.round < scenario.importer.lastblock:
            logger.error(f"conduit did not reach round={scenario.importer.lastblock}")
            sys.stderr.write(indexerout.dump())
            return 1

        # now indexer's round == the final network round
        if errors := scenario.get_validation_errors():
            logger.error(f"conduit failed validation: {errors}")
            sys.stderr.write(indexerout.dump())
            return 1

        logger.info(
            f"reached expected round={scenario.importer.lastblock} and passed validation"
        )
        dt = time.time() - start
        sys.stdout.write("conduit e2etest OK ({:.1f}s)\n".format(dt))
        return 0
