"""A setup abort must never manufacture a measurement-relative receipt cutoff."""
import json
import pathlib
import subprocess
import sys
import tempfile
import unittest

ROOT=pathlib.Path(__file__).resolve().parents[1]


class CaptureCutoffTests(unittest.TestCase):
    def derive(self, epoch):
        source=(ROOT/'scripts/run-protocol-capture.sh').read_text()
        block=source[source.index('cutoff_ms=$('):]
        program=block.split("<<'PY'\n",1)[1].split('\nPY\n)',1)[0]
        with tempfile.TemporaryDirectory() as d:
            p=pathlib.Path(d)
            values=[{'steps':[{'index':1,'name':'measure','scenario_start_ms':epoch}]},
                    {'protocol_pilot_parameters':{'drain_seconds_after_measurement':900}},
                    {'steps':[{'index':1,'name':'measure','phase':'measurement','duration_s':600}]}]
            paths=[]
            for i,value in enumerate(values):
                path=p/f'{i}.json';path.write_text(json.dumps(value));paths.append(str(path))
            return subprocess.run([sys.executable,'-c',program,*paths,'90000'],capture_output=True,text=True)

    def test_actual_measurement_epoch_produces_frozen_relative_cutoff(self):
        p=self.derive(100000)
        self.assertEqual(p.returncode,0,p.stderr)
        self.assertEqual(float(p.stdout),1600000)

    def test_setup_abort_or_invalid_epoch_has_no_cutoff(self):
        for epoch in (0,None,-1,float('nan')):
            with self.subTest(epoch=epoch):
                p=self.derive(epoch)
                self.assertNotEqual(p.returncode,0)
                self.assertIn('measurement scenario did not start',p.stderr)
