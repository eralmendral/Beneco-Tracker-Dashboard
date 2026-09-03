import tempfile
import unittest
from pathlib import Path

from pdf_cells import extract_cell_text
from scrape_beneco import discover_aep_pdf_url, write_json


class FakePage:
    def __init__(self, chars):
        self.chars = chars


def char(text, x0, x1, top=10, bottom=15):
    return {"text": text, "x0": x0, "x1": x1, "top": top, "bottom": bottom}


class ScraperTests(unittest.TestCase):
    def test_discovers_accredited_practitioner_pdf(self):
        html = """
        <a href="other.pdf">Other form</a>
        <a href="forms/aep%20list.pdf">
          List of Accredited Electrical Practitioners
        </a>
        """

        self.assertEqual(
            discover_aep_pdf_url(html, "https://example.test/forms.php"),
            "https://example.test/forms/aep%20list.pdf",
        )

    def test_missing_accredited_practitioner_link_is_an_error(self):
        with self.assertRaisesRegex(ValueError, "could not find"):
            discover_aep_pdf_url('<a href="other.pdf">Other form</a>')

    def test_cell_extraction_excludes_characters_centered_outside_band(self):
        page = FakePage([
            char("X", 8.5, 10.5),
            char("A", 10.5, 12.5),
            char("B", 13.0, 15.0),
            char("Y", 19.5, 20.5),
        ])

        self.assertEqual(extract_cell_text(page, (10, 0, 20, 20)), "AB")

    def test_cell_extraction_joins_wrapped_lines(self):
        page = FakePage([
            char("A", 10, 12),
            char("B", 14, 16),
            char("C", 10, 12, top=20, bottom=25),
        ])

        self.assertEqual(extract_cell_text(page, (0, 0, 30, 30)), "A B C")

    def test_write_json_replaces_snapshot(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "data.json"
            write_json(path, [{"barangayid": 1, "barangay": "Abiang"}])

            self.assertEqual(
                path.read_text(encoding="utf-8"),
                '[\n  {\n    "barangayid": 1,\n    "barangay": "Abiang"\n  }\n]\n',
            )


if __name__ == "__main__":
    unittest.main()
