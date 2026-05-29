import unittest
from importlib import util
from io import BytesIO
from pathlib import Path
import sys
import types


def _load_deep_parser_module():
    try:
        from docreader.parser import deep_parser_parser

        return deep_parser_parser
    except ModuleNotFoundError as exc:
        if exc.name != "textract":
            raise

    docreader_dir = Path(__file__).resolve().parents[1]
    parser_pkg = types.ModuleType("docreader.parser")
    parser_pkg.__path__ = [str(docreader_dir / "parser")]
    sys.modules.pop("docreader.parser", None)
    sys.modules["docreader.parser"] = parser_pkg

    models_pkg = types.ModuleType("docreader.models")
    models_pkg.__path__ = [str(docreader_dir / "models")]
    document_module = types.ModuleType("docreader.models.document")

    class Document:
        def __init__(self, content="", images=None, metadata=None):
            self.content = content
            self.images = images or {}
            self.metadata = metadata or {}

    document_module.Document = Document
    sys.modules["docreader.models"] = models_pkg
    sys.modules["docreader.models.document"] = document_module

    module_name = "docreader.parser.deep_parser_parser"
    spec = util.spec_from_file_location(module_name, docreader_dir / "parser" / "deep_parser_parser.py")
    module = util.module_from_spec(spec)
    sys.modules[module_name] = module
    spec.loader.exec_module(module)
    return module


deep_parser_module = _load_deep_parser_module()
DeepParserParser = deep_parser_module.DeepParserParser
UploadPart = deep_parser_module.UploadPart


class RoutingDeepParser(DeepParserParser):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.calls = []

    def _render_pdf_to_images(self, content: bytes):
        self.calls.append(("pdf2image", len(content)))
        return [UploadPart("doc-p0001.png", "image/png", b"pdf-page")]

    def _convert_to_images(self, content: bytes):
        self.calls.append(("converter", len(content)))
        return [UploadPart("doc-p0001.png", "image/png", b"converted-page")]

    def _call_deep_parser(self, upload_parts):
        self.calls.append(("deep_parser", [part.filename for part in upload_parts]))
        return {
            "total_pages": len(upload_parts),
            "markdown_pages": [{"page_number": 1, "content": "ok"}],
        }


class DeepParserParserRoutingTest(unittest.TestCase):
    def test_pdf_uses_builtin_pdf_image_renderer_before_deep_parser(self):
        parser = RoutingDeepParser(file_name="sample.pdf", file_type="pdf")

        document = parser.parse_into_text(b"%PDF-1.7")

        self.assertEqual([call[0] for call in parser.calls], ["pdf2image", "deep_parser"])
        self.assertEqual(document.metadata["deep_parser_image_source"], "pdf2image")
        self.assertIn("deep_office_page_images", document.metadata)
        self.assertIn("deep-office-page-image://1/doc-p0001.png", document.images)
        self.assertNotIn("converter_endpoint", document.metadata)

    def test_office_files_use_converter_before_deep_parser(self):
        parser = RoutingDeepParser(file_name="proposal.docx", file_type="docx")

        document = parser.parse_into_text(b"docx bytes")

        self.assertEqual([call[0] for call in parser.calls], ["converter", "deep_parser"])
        self.assertEqual(document.metadata["deep_parser_image_source"], "converter")
        self.assertIn("deep_office_page_images", document.metadata)
        self.assertIn("deep-office-page-image://1/doc-p0001.png", document.images)
        self.assertIn("converter_endpoint", document.metadata)

    def test_images_are_converted_to_png_and_sent_directly_to_deep_parser(self):
        from PIL import Image

        source = BytesIO()
        Image.new("RGB", (2, 2), (255, 0, 0)).save(source, format="JPEG")
        parser = RoutingDeepParser(file_name="page.jpg", file_type="jpg")

        document = parser.parse_into_text(source.getvalue())

        self.assertEqual(parser.calls, [("deep_parser", ["page.png"])])
        self.assertEqual(document.metadata["deep_parser_image_source"], "image_to_png")
        self.assertIn("deep-office-page-image://1/page.png", document.images)


if __name__ == "__main__":
    unittest.main()
