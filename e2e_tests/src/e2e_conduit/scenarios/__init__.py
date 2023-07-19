from __future__ import annotations

from dataclasses import dataclass, field
from typing import Callable

from e2e_conduit.fixtures.importers.importer_plugin import ImporterPlugin
from e2e_conduit.fixtures.plugin_fixture import PluginFixture


@dataclass
class Scenario:
    """Data class for conduit E2E test pipelines"""
    name: str
    importer: ImporterPlugin
    processors: list[PluginFixture]
    exporter: PluginFixture
    accumulated_config: dict = field(default_factory=dict)
    conduit_dir: str = ""

    def validate(self) -> list[str]:
        """
        Validate the scenario. 
        A non empty list of error messages signals a failed validation.
        """
        return []

scenarios: list[Scenario] = []
