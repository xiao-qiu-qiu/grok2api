#!/usr/bin/env python3

import tempfile
import unittest
from pathlib import Path

from from_residential import (
    assign_roles,
    guard_defaults,
    main,
    parse_dump,
    parse_proxy_line,
    split_roles,
)


DUMP = """
# comment
use-a | http://alice:secret@res.example:3000
bob:pass@res.example:3001
res.example:3002:carol:pw
socks5://dave:pw@res.example:1080
http://acct-region-US-sid-AAA111-t-10:token@us.1024proxy.example:10000
http://acct-region-US-sid-BBB222-t-10:token@us.1024proxy.example:10000
"""


class ParseTests(unittest.TestCase):
    def test_url_and_aliases(self):
        a = parse_proxy_line("use-a | http://alice:secret@res.example:3000")
        self.assertEqual(a["name"], "use-a")
        self.assertEqual(a["host"], "res.example")
        self.assertEqual(a["username"], "alice")

        b = parse_proxy_line("bob:pass@res.example:3001")
        self.assertEqual(b["host"], "res.example")
        self.assertEqual(b["port"], 3001)

        c = parse_proxy_line("res.example:3002:carol:pw")
        self.assertEqual(c["username"], "carol")
        self.assertEqual(c["password"], "pw")

        d = parse_proxy_line("socks5://dave:pw@res.example:1080")
        self.assertEqual(d["scheme"], "socks5")

        e = parse_proxy_line(
            "http://acct-region-US-sid-AAA111-t-10:token@us.1024proxy.example:10000"
        )
        self.assertEqual(e["sid"], "AAA111")

    def test_rejects_duplicate_and_empty(self):
        with self.assertRaises(ValueError):
            parse_dump("http://a:b@h:1\nhttp://a:b@h:1\n")
        with self.assertRaises(ValueError):
            parse_dump("# only comments\n")

    def test_split_and_guard(self):
        self.assertEqual(split_roles(1), (1, 0))
        self.assertEqual(split_roles(3), (3, 0))
        self.assertEqual(split_roles(4), (3, 1))
        self.assertEqual(split_roles(8), (6, 2))

        one = guard_defaults(1)
        self.assertFalse(one["lab_like"])
        self.assertFalse(one["fail_closed"])
        self.assertEqual(one["min_healthy_nodes"], 1)

        three = guard_defaults(3)
        self.assertTrue(three["lab_like"])
        self.assertTrue(three["fail_closed"])
        self.assertEqual(three["min_healthy_nodes"], 2)
        self.assertEqual(three["soft_tps"], 200)

        four = guard_defaults(4)
        self.assertEqual(four["min_healthy_nodes"], 3)

    def test_assign_keeps_every_session(self):
        sessions = parse_dump(DUMP)
        self.assertEqual(len(sessions), 6)
        nodes = assign_roles(sessions)
        self.assertEqual(len(nodes), 6)
        use = [n for n in nodes if n["role"] == "use"]
        reg = [n for n in nodes if n["role"] == "reg"]
        self.assertEqual(len(use), 5)
        self.assertEqual(len(reg), 1)
        self.assertEqual(use[0]["listen_port"], 8301)
        self.assertEqual(reg[0]["listen_port"], 8201)
        names = {n["proxy_name"] for n in nodes}
        self.assertEqual(len(names), 6)

    def test_cli_writes_files(self):
        with tempfile.TemporaryDirectory() as tmp:
            dump = Path(tmp) / "dump.txt"
            dump.write_text(DUMP, encoding="utf-8")
            out = Path(tmp) / "gen"
            self.assertEqual(main([str(dump), "--out-dir", str(out)]), 0)
            yaml = (out / "mihomo.yaml").read_text(encoding="utf-8")
            self.assertIn("sticky-use-01", yaml)
            self.assertIn("port: 8301", yaml)
            self.assertNotIn("alice:secret@res.example", (out / "nodes.json").read_text())
            plan = (out / "plan.md").read_text(encoding="utf-8")
            self.assertIn("use-side nodes: 5", plan)


if __name__ == "__main__":
    unittest.main()
