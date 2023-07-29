from datetime import datetime, timedelta
import gzip
import io
import logging
import re

logger = logging.getLogger(__name__)

# Matches conduit log output:
# "FINISHED Pipeline round: 110. UPDATED Pipeline round: 111"
FINISH_ROUND: re.Pattern = re.compile(
    b"FINISHED Pipeline round: (\d+). UPDATED Pipeline round"
)

# Matches error from attempting to sync past network's final round:
# "waitForRoundWithTimeout: wrong round returned from status for round: 102 != 103: status2.LastRound mismatch: context deadline exceeded"
# END_OF_TEST_SYNC_MISMATCH: re.Pattern = re.compile(
#     b"wrong round returned from status for round: (\d+) != (\d+).*context deadline exceeded"
# )


class subslurp:
    """accumulate stdout or stderr from a subprocess and hold it for debugging if something goes wrong"""

    def __init__(self, f):
        self.f = f
        self.buf = io.BytesIO()
        self.gz = gzip.open(self.buf, "wb")
        self.timeout = timedelta(seconds=120)
        self.round = 0
        self.error_log = None

    # def is_log_error(self, log_line):
    #     if b"error" in log_line:
    #         # match = re.search(END_OF_TEST_SYNC_MISMATCH, log_line)
    #         # if match:
    #         #     x, y = list(map(int, match.groups()))
    #         #     if x + 1 == y and x == lastround:
    #         #         return False

    #         self.error_log = log_line
    #         return True
    #     return False

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
                    # NOTE this quite very strict criterion!!!
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
