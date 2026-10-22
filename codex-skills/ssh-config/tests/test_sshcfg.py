# TESTMASTER: id=sshbak tier=fast parallel=yes
"""Cases sshbak-1…12 (requirement req-lapwing-ssh-config-backups).

Every case builds its own fixture: a config whose Include names an absolute
config.d, because ssh expands ~ from the passwd entry, not $HOME. Run with
`python3 -m unittest skills/ssh-config/tests/test_sshcfg.py` from the repo root.
"""
import os
import shutil
import subprocess
import sys
import tempfile
import unittest

HELPER = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "scripts", "sshcfg.py")
HAVE_SSH = shutil.which("ssh") is not None

ED_HOUSE = "# ya' know, ed's grarrage\n\nHost explorer\n  HostName 10.0.0.1\n  LocalForward 7778 localhost:7402\n\nHost wintest\n  HostName 10.0.0.2\n"
CYPRESS = "Host bryanHouse\n  HostName 192.168.1.5\n"


class Fixture:
    def __init__(self):
        self.root = tempfile.mkdtemp(prefix="sshcfg-")
        self.d = os.path.join(self.root, "config.d")
        self.backups = self.d + "-backups"
        os.makedirs(self.d)
        self.config = os.path.join(self.root, "config")
        with open(self.config, "w") as f:
            f.write(f"Include {self.d}/*\n")
        self.put("ed-house", ED_HOUSE)
        self.put("cypress", CYPRESS)

    def put(self, name, text):
        with open(os.path.join(self.d, name), "w") as f:
            f.write(text)

    def read(self, name):
        with open(os.path.join(self.d, name)) as f:
            return f.read()

    def run(self, *args):
        p = subprocess.run([sys.executable, HELPER, *args, "--config", self.config],
                           capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def new_version(self, site, text):
        code, out = self.run("tempfile", site)
        path = out.strip()
        with open(path, "w") as f:
            f.write(text)
        return path

    def glob_names(self):
        return sorted(os.listdir(self.d))

    def snapshot(self):
        snap = {}
        for base in (self.d, self.backups):
            if os.path.isdir(base):
                for n in os.listdir(base):
                    p = os.path.join(base, n)
                    with open(p, "rb") as f:
                        snap[p] = f.read()
        return snap

    def cleanup(self):
        shutil.rmtree(self.root, ignore_errors=True)


@unittest.skipUnless(HAVE_SSH, "ssh is not installed")
class SshcfgCases(unittest.TestCase):
    def setUp(self):
        self.fx = Fixture()
        self.addCleanup(self.fx.cleanup)

    def assert_glob_clean(self):
        names = self.fx.glob_names()
        bad = [n for n in names if n.endswith(".bak.skill") or n.endswith(".tmp") or n.endswith(".new") or n.startswith(".")]
        self.assertEqual(bad, [], f"config.d holds non-site files: {names}")

    def test_sshbak_1_add_backs_up_outside_the_glob(self):
        new = self.fx.new_version("ed-house", ED_HOUSE + "\nHost newbox\n  HostName 10.0.0.9\n")
        self.assertTrue(new.startswith(self.fx.backups), f"temp file {new} is inside the include glob")
        code, out = self.fx.run("write", "ed-house", new, "--verify", "newbox")
        self.assertEqual(code, 0, out)
        self.assert_glob_clean()
        backups = [n for n in os.listdir(self.fx.backups) if n.startswith("ed-house.")]
        self.assertEqual(len(backups), 1, f"expected one timestamped backup, got {backups}")
        with open(os.path.join(self.fx.backups, backups[0])) as f:
            self.assertEqual(f.read(), ED_HOUSE, "backup must hold the pre-write file")
        self.assertIn("Host newbox", self.fx.read("ed-house"))

    def test_sshbak_2_every_write_path_backs_up_outside_the_glob(self):
        # delete (ed-house loses wintest), move (wintest goes to cypress), sort (every site), speedup.
        steps = [("ed-house", ED_HOUSE.replace("\nHost wintest\n  HostName 10.0.0.2\n", "")),
                 ("cypress", CYPRESS + "\nHost wintest\n  HostName 10.0.0.2\n"),
                 ("cypress", "Host bryanHouse\n  HostName 192.168.1.5\n\nHost wintest\n  HostName 10.0.0.2\n"),
                 ("ed-house", ED_HOUSE.replace("HostName 10.0.0.1\n", "HostName 10.0.0.1\n  ControlMaster auto\n  ControlPath ~/.ssh/cm-%C\n  ControlPersist 600\n"))]
        for site, text in steps:
            code, out = self.fx.run("write", site, self.fx.new_version(site, text))
            self.assertEqual(code, 0, out)
            self.assert_glob_clean()
        names = sorted(os.listdir(self.fx.backups))
        self.assertEqual(sum(n.startswith("ed-house.") for n in names), 2, names)
        self.assertEqual(sum(n.startswith("cypress.") for n in names), 2, names)

    def test_sshbak_3_list_writes_nothing(self):
        before = self.fx.snapshot()
        for verb in (("sites",), ("dupes",), ("dupes", "ed-house")):
            code, out = self.fx.run(*verb)
            self.assertEqual(code, 0, out)
        self.assertEqual(self.fx.snapshot(), before, "a read-only verb changed config.d or the backups")
        self.assertFalse(os.path.exists(self.fx.backups), "listing created the backups directory")

    def test_sshbak_4_verify_names_every_loaded_copy(self):
        copies = ["ed-house.bak.skill", "ed-house~", "ed-house.bak.20260628", "ed-house.sync-conflict-20261001-1234", "ed-house.tmp"]
        for name in copies:
            self.fx.put(name, "Host explorer\n  LocalForward 7777 localhost:7402\n")
        code, out = self.fx.run("verify", "explorer")
        self.assertEqual(code, 1, out)
        for name in copies:
            self.assertIn(os.path.join(self.fx.d, name), out, f"verify did not name {name}")
        self.assertIn("APPLIES", out, "verify must say which copy applied options to the host")

    def test_sshbak_5_live_sites_and_groupings_pass(self):
        self.fx.put("1", "")
        self.fx.put("octa", "")
        self.fx.put("gke", "Host gke-node\n  HostName 10.1.0.1\n")
        self.fx.put("incus-uc-vms", "Host uc1\n  HostName 10.2.0.1\n")
        code, out = self.fx.run("verify", "explorer")
        self.assertEqual(code, 0, out)
        self.assertIn("options come only from live site files", out)

    def test_sshbak_6_old_is_flagged_because_ssh_loads_it(self):
        self.fx.put("old", "Host explorer\n  User someone-else\n")
        code, out = self.fx.run("verify", "explorer")
        self.assertEqual(code, 1, out)
        self.assertIn(os.path.join(self.fx.d, "old"), out)

    def test_sshbak_7_duplicate_hosts_within_a_site_name_both_lines(self):
        self.fx.put("ed-house", ED_HOUSE + "\nHost explorer\n  HostName 10.0.0.3\n")
        code, out = self.fx.run("dupes", "ed-house")
        self.assertEqual(code, 0, out)
        self.assertRegex(out, r"DUPLICATE\ted-house\tHost explorer\tlines 3, 10")

    def test_sshbak_8_no_duplicates_prints_none(self):
        code, out = self.fx.run("dupes")
        self.assertEqual(code, 0, out)
        self.assertNotIn("DUPLICATE", out)
        self.assertNotIn("CROSS-SITE", out)
        self.assertIn("no duplicate Host names", out)

    def test_sshbak_9_cross_site_duplicates_on_their_own_line(self):
        self.fx.put("cypress", CYPRESS + "\nHost explorer\n  HostName 10.9.9.9\n")
        code, out = self.fx.run("dupes")
        self.assertEqual(code, 0, out)
        self.assertIn("CROSS-SITE\tHost explorer\t", out)
        self.assertNotIn("DUPLICATE", out)

    def test_sshbak_10_failed_verify_restores_byte_identical(self):
        before = self.fx.read("ed-house")
        broken = self.fx.new_version("ed-house", ED_HOUSE + "\nHost broken\n  NoSuchOption yes\n")
        code, out = self.fx.run("write", "ed-house", broken, "--verify", "explorer")
        self.assertEqual(code, 3, out)
        self.assertEqual(self.fx.read("ed-house"), before, "the site was not restored byte-identical")
        self.assert_glob_clean()

    def test_sshbak_11_interrupted_write_leaves_nothing_loadable(self):
        before = self.fx.glob_names()
        self.fx.new_version("ed-house", "Host half\n")  # staged, never installed: the write was interrupted
        self.assertEqual(self.fx.glob_names(), before, "a staged temp file appeared inside the include glob")
        code, out = self.fx.run("verify", "explorer")
        self.assertEqual(code, 0, out)
        self.fx.put("ed-house.tmp", "Host explorer\n  LocalForward 7777 localhost:7402\n")
        code, out = self.fx.run("audit")
        self.assertEqual(code, 1, out)
        self.assertIn(os.path.join(self.fx.d, "ed-house.tmp"), out)

    def test_sshbak_12_migrate_moves_skill_backups_out(self):
        self.fx.put("ed-house.bak.skill", "Host explorer\n  LocalForward 7777 localhost:7402\n")
        code, out = self.fx.run("migrate")
        self.assertEqual(code, 0, out)
        self.assertNotIn("ed-house.bak.skill", self.fx.glob_names())
        self.assertTrue(any(n.endswith(".migrated") for n in os.listdir(self.fx.backups)))
        p = subprocess.run(["ssh", "-G", "-F", self.fx.config, "explorer"], capture_output=True, text=True)
        forwards = [l for l in p.stdout.splitlines() if l.startswith("localforward")]
        self.assertEqual(forwards, ["localforward 7778 [localhost]:7402"], f"ssh still loads the stale backup: {forwards}")

    def test_write_refuses_a_source_inside_the_glob(self):
        self.fx.put("ed-house.new", ED_HOUSE)
        code, out = self.fx.run("write", "ed-house", os.path.join(self.fx.d, "ed-house.new"))
        self.assertEqual(code, 2, out)
        self.assertIn("inside the include glob", out)

    def test_backups_keep_the_newest_twenty(self):
        for i in range(23):
            code, out = self.fx.run("backup", "ed-house")
            self.assertEqual(code, 0, out)
        self.assertEqual(len([n for n in os.listdir(self.fx.backups) if n.startswith("ed-house.")]), 20)


if __name__ == "__main__":
    unittest.main()
