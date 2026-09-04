import json
import tempfile
import unittest
from pathlib import Path

from pdf_cells import extract_cell_text
from scrape_beneco import (
    discover_aep_pdf_url,
    discover_facebook_embed_url,
    discover_facebook_timeline_post_urls,
    normalize_feeders,
    parse_facebook_reports,
    parse_graph_facebook_reports,
    write_json,
)


class FakePage:
    def __init__(self, chars):
        self.chars = chars


class FakeResponse:
    def __init__(self, text):
        self.text = text

    def raise_for_status(self):
        return None


class FakeFacebookSession:
    def __init__(self):
        self.calls = []

    def get(self, url, **kwargs):
        self.calls.append((url, kwargs))
        if "plugins/page.php" in url:
            return FakeResponse('<script>["LSD",[],{"token":"public-token"}]</script>')
        return FakeResponse(
            r'{"markup":"https:\/\/www.facebook.com\/benguetelectric\/posts\/pfbid123?ref=embed_page '
            r'https:\/\/www.facebook.com\/benguetelectric\/videos\/456\/"}'
        )


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

    def test_normalizes_single_and_multi_feeder_labels(self):
        self.assertEqual(normalize_feeders("Feeder 08"), ["FEEDER_08"])
        self.assertEqual(
            normalize_feeders("Feeders 11 12 14"),
            ["FEEDER_11", "FEEDER_12", "FEEDER_14"],
        )
        self.assertEqual(normalize_feeders("Circuit 3"), ["CIRCUIT_03"])
        self.assertEqual(normalize_feeders("Dalicno"), ["FEEDER_DALICNO"])
        self.assertEqual(normalize_feeders("Feeder 8 since 3am"), ["FEEDER_08"])

    def test_discovers_official_facebook_embed(self):
        source = '<iframe src="https://www.facebook.com/plugins/post.php?href=post&amp;width=500"></iframe>'
        self.assertEqual(
            discover_facebook_embed_url(source),
            "https://www.facebook.com/plugins/post.php?href=post&width=500",
        )

    def test_facebook_report_is_matched_with_anonymous_comment_excerpt(self):
        post = {
            "@type": "SocialMediaPosting",
            "url": "https://www.facebook.com/benguetelectric/posts/123/",
            "comment": [{
                "identifier": "comment-1",
                "dateCreated": "2026-09-02T03:57:35-0700",
                "text": "Woodsgate Subdivision since Tuesday 3am hanggang ngayon walang kuryente",
                "author": {"name": "Private Name"},
            }],
        }
        embed = f'<script type="application/ld+json">{json.dumps(post)}</script>'
        outages = [{"feeder": "FEEDER_12", "area": "Parts of Woodsgate tapped at Pelota Court"}]

        reports = parse_facebook_reports(embed, outages, [])

        self.assertEqual(len(reports), 1)
        self.assertEqual(reports[0]["feeder"], "FEEDER_12")
        self.assertEqual(reports[0]["category"], "outage")
        self.assertEqual(reports[0]["location"], "Woodsgate Subdivision")
        self.assertEqual(
            reports[0]["comment_excerpt"],
            "Woodsgate Subdivision since Tuesday 3am hanggang ngayon walang kuryente",
        )
        self.assertNotIn("author", reports[0])
        self.assertNotIn("text", reports[0])

    def test_unmapped_complaint_is_kept_and_contact_details_are_redacted(self):
        post = {
            "@type": "SocialMediaPosting",
            "url": "https://www.facebook.com/benguetelectric/posts/456/",
            "comment": [{
                "identifier": "comment-2",
                "dateCreated": "2026-09-03T10:15:00+08:00",
                "text": "Wala pa rin kuryente, call me at 09171234567 or test@example.com",
            }],
        }
        embed = f'<script type="application/ld+json">{json.dumps(post)}</script>'

        reports = parse_facebook_reports(embed, [], [])

        self.assertEqual(len(reports), 1)
        self.assertEqual(reports[0]["feeder"], "UNMAPPED")
        self.assertIn("[number removed]", reports[0]["comment_excerpt"])
        self.assertIn("[email removed]", reports[0]["comment_excerpt"])
        self.assertNotIn("09171234567", reports[0]["comment_excerpt"])

    def test_billing_complaint_is_retained_without_a_feeder_match(self):
        post = {
            "@type": "SocialMediaPosting",
            "url": "https://www.facebook.com/benguetelectric/posts/654/",
            "comment": [{
                "identifier": "comment-billing",
                "dateCreated": "2026-09-04T08:00:00+08:00",
                "text": "Ask lang po bakit biglang tumaas ang bill namin this month?",
            }],
        }
        embed = f'<script type="application/ld+json">{json.dumps(post)}</script>'

        reports = parse_facebook_reports(embed, [], [])

        self.assertEqual(len(reports), 1)
        self.assertEqual(reports[0]["category"], "billing")
        self.assertEqual(reports[0]["feeder"], "UNMAPPED")

    def test_ilocano_outage_complaint_and_explicit_location_are_retained(self):
        post = {
            "@type": "SocialMediaPosting",
            "url": "https://www.facebook.com/benguetelectric/posts/655/",
            "comment": [{
                "identifier": "comment-ilocano",
                "dateCreated": "2026-09-04T08:00:00+08:00",
                "text": "3days nga awan pay kuryente dtoy Kennon!",
            }],
        }
        embed = f'<script type="application/ld+json">{json.dumps(post)}</script>'

        reports = parse_facebook_reports(embed, [], [])

        self.assertEqual(len(reports), 1)
        self.assertEqual(reports[0]["category"], "outage")
        self.assertEqual(reports[0]["location"], "Kennon")
        self.assertEqual(reports[0]["feeder"], "UNMAPPED")

    def test_discovers_recent_public_timeline_post_links(self):
        session = FakeFacebookSession()

        urls = discover_facebook_timeline_post_urls(session, 10)

        self.assertEqual(urls, [
            "https://www.facebook.com/benguetelectric/posts/pfbid123",
            "https://www.facebook.com/benguetelectric/videos/456",
        ])
        self.assertEqual(len(session.calls), 2)

    def test_graph_comments_are_normalized_for_multiple_page_posts(self):
        payload = {"data": [{
            "id": "post-1",
            "permalink_url": "https://www.facebook.com/benguetelectric/posts/789/",
            "comments": {"data": [{
                "id": "comment-3",
                "created_time": "2026-09-03T12:00:00+0800",
                "message": "Camp 7 no power since this morning",
                "from": {"name": "Not Retained"},
            }]},
        }]}
        barangays = [{"barangay": "Camp 7", "feeder": "FEEDER_12"}]

        reports = parse_graph_facebook_reports(payload, [], barangays)

        self.assertEqual(len(reports), 1)
        self.assertEqual(reports[0]["source_id"], "comment-3")
        self.assertEqual(reports[0]["feeder"], "FEEDER_12")
        self.assertEqual(reports[0]["category"], "outage")
        self.assertNotIn("from", reports[0])

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
