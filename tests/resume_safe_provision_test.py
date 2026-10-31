"""Safety regression: qualification preparation must never reset retained volumes."""
import pathlib
import unittest


class ResumeSafeTest(unittest.TestCase):
    def test_prepare_preserves_storage(self):
        path = pathlib.Path(__file__).parents[1]/'deploy/edgex/scripts/provision-then-scale.sh'
        text = path.read_text()
        prepare = text.split('  prepare)', 1)[1].split('  check)', 1)[0]
        self.assertNotIn('down -v', prepare)
        self.assertNotIn('volume rm', prepare)
        self.assertIn('EDGEX_REPLICAS=1 docker compose up -d', prepare)


if __name__ == '__main__':
    unittest.main()
