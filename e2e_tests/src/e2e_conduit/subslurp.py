from datetime import datetime, timedelta
import gzip
import io
import logging
import re

logger = logging.getLogger(__name__)

# Matches conduit log output:
# "FINISHED Pipeline round r=42 (13 txn) exported in 12.3456s"
FINISH_ROUND: re.Pattern = re.compile(b"FINISHED Pipeline round r=(\d+)")


class subslurp:
    """accumulate stdout or stderr from a subprocess and hold it for debugging if something goes wrong"""

    def __init__(self, f):
        self.f = f
        self.buf = io.BytesIO()
        self.gz = gzip.open(self.buf, "wb")
        self.timeout = timedelta(seconds=120)
        self.round = 0
        self.error_log = None

    def tryParseRound(self, log_line):
        match = FINISH_ROUND.search(log_line)
        if match and (r := int(match.group(1))) is not None:
            self.round = r

    def run(self, lastround):
        if len(self.f.peek().strip()) == 0:
            logger.info("No Conduit output found")
            return

        start = datetime.now()
        lastlog = datetime.now()
        while (
            datetime.now() - start < self.timeout
            and datetime.now() - lastlog < timedelta(seconds=15)
        ):
            for i, line in enumerate(self.f):
                lastlog = datetime.now()
                if self.gz is not None:
                    self.gz.write(line)
                self.tryParseRound(line)
                if self.round >= lastround:
                    logger.info(
                        f"Conduit reached desired lastround after parsing line {i+1}: {lastround}"
                    )
                    return
                if b"error" in line:
                    # NOTE this quite strict criterion!!!
                    raise RuntimeError(
                        f"E2E tests logged an error at line {i+1}: {self.error_log}"
                    )

    def dump(self) -> str:
        if self.gz is not None:
            self.gz.close()
        self.gz = None
        self.buf.seek(0)
        r = gzip.open(self.buf, "rt")
        return r.read()  # type: ignore
