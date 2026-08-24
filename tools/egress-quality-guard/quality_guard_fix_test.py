import importlib.util
import sys
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("quality_guard.py")
SPEC = importlib.util.spec_from_file_location("quality_guard_fix", MODULE_PATH)
quality_guard = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
sys.modules[SPEC.name] = quality_guard
SPEC.loader.exec_module(quality_guard)


def make_config():
    return quality_guard.Config(
        base_url="http://grok2api:8000",
        internal_token="test-token",
        model="grok-4.6",
        node_ids=("4",),
        mode="passive",
        active_interval_seconds=1800,
        passive_poll_seconds=5,
        passive_page_size=200,
        passive_max_pages=10,
        jitter_seconds=0,
        request_timeout_seconds=120,
        soft_tps=500.0,
        hard_tps=2500.0,
        consecutive_soft=2,
        consecutive_errors=2,
        quarantine_seconds=300,
        no_account_backoff_seconds=300,
        min_healthy_nodes=1,
        max_output_tokens=384,
        fail_closed=False,
        enabled=True,
        quarantine_enabled=True,
        min_generation_ms=1000,
        rotation_url="",
        rotation_token="",
        rotation_timeout_seconds=45,
        rotatable_node_ids=(),
        prompt="probe",
        expected="QUALITY_OK",
        state_file=Path("/tmp/quality-guard-fix-state.json"),
        lock_file=Path("/tmp/quality-guard-fix.lock"),
        runtime_config_file=Path("/tmp/quality-guard-fix-runtime.json"),
    )


class GenerationWindowTests(unittest.TestCase):
    def test_reasoning_short_tail_uses_full_duration(self):
        self.assertEqual(quality_guard.generation_window_ms(5379, 5418, 15), 5418)
        classification, reason, speed, output = quality_guard.classify_audit(
            {
                "provider": "grok_build",
                "streaming": True,
                "statusCode": 200,
                "firstTokenMs": 5379,
                "durationMs": 5418,
                "outputTokens": 59,
                "reasoningTokens": 15,
            },
            make_config(),
        )
        self.assertEqual((classification, reason, output), ("healthy", "within_threshold", 59))
        self.assertAlmostEqual(speed, 59 * 1000 / 5418)

    def test_successful_missing_thinking_is_still_classified(self):
        classification, reason, speed, output = quality_guard.classify_audit(
            {
                "provider": "grok_build",
                "streaming": True,
                "statusCode": 200,
                "firstTokenMs": 1000,
                "durationMs": 5000,
                "outputTokens": 64,
                "reasoningTokens": 0,
            },
            make_config(),
        )
        self.assertEqual((classification, reason, output), ("hard", "missing_thinking", 64))
        self.assertEqual(speed, 16)


if __name__ == "__main__":
    unittest.main()
