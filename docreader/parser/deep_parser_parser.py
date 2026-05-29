"""DeepParser parser engine for WeKnora DocReader."""

from __future__ import annotations

import base64
import hashlib
import json
import logging
import mimetypes
import os
import urllib.error
import urllib.parse
import urllib.request
import uuid
import zipfile
from dataclasses import dataclass
from io import BytesIO
from typing import Any

from docreader.config import CONFIG
from docreader.models.document import Document
from docreader.parser.base_parser import BaseParser
from docreader.parser.concurrency import parser_worker_limit

logger = logging.getLogger(__name__)

DEEP_PARSER_ENGINE = "deep_parser"
DEFAULT_CONVERTER_ENDPOINT = "http://192.168.20.60:2223/convert"
DEFAULT_PARSER_ENDPOINT = "http://192.168.20.60:9888/parse/files_new"
DEFAULT_PARSER_MODE = "ultra_mineru"
DIRECT_IMAGE_TYPES = {"jpg", "jpeg", "png", "gif", "bmp", "tiff", "webp"}
CONVERTER_TYPES = {"doc", "docx", "ppt", "pptx", "hwp", "hwpx"}
DEEP_OFFICE_PAGE_IMAGE_REF_PREFIX = "deep-office-page-image://"


@dataclass(frozen=True)
class UploadPart:
    filename: str
    content_type: str
    content: bytes


class DeepParserParser(BaseParser):
    """Convert layout-heavy documents to page images, then call DeepParser."""

    def __init__(self, file_name: str = "", file_type: str | None = None, **kwargs: Any):
        super().__init__(file_name=file_name, file_type=file_type, **kwargs)
        self.converter_endpoint = _endpoint_url(
            str(
                kwargs.get("deep_parser_converter_endpoint")
                or kwargs.get("converter_endpoint")
                or os.environ.get("DEEP_PARSER_CONVERTER_ENDPOINT")
                or DEFAULT_CONVERTER_ENDPOINT
            )
        )
        self.parser_endpoint = _endpoint_url(
            str(
                kwargs.get("deep_parser_endpoint")
                or kwargs.get("parser_endpoint")
                or os.environ.get("DEEP_PARSER_ENDPOINT")
                or DEFAULT_PARSER_ENDPOINT
            )
        )
        self.mode = str(kwargs.get("mode") or os.environ.get("DEEP_PARSER_MODE") or DEFAULT_PARSER_MODE)
        self.width = int(kwargs.get("convert_width") or os.environ.get("DEEP_PARSER_CONVERT_WIDTH") or "2560")
        self.height = int(kwargs.get("convert_height") or os.environ.get("DEEP_PARSER_CONVERT_HEIGHT") or "2560")
        self.timeout = float(kwargs.get("timeout") or os.environ.get("DEEP_PARSER_TIMEOUT") or "900")

    def parse_into_text(self, content: bytes) -> Document:
        upload_parts, image_source = self._upload_parts_for_content(content)
        payload = self._call_deep_parser(upload_parts)
        return self._document_from_deep_parser_response(payload, upload_parts, image_source)

    def _upload_parts_for_content(self, content: bytes) -> tuple[list[UploadPart], str]:
        file_type = (self.file_type or "").lower().lstrip(".")
        if file_type in DIRECT_IMAGE_TYPES:
            return [self._image_to_png(content)], "image_to_png"
        if file_type == "pdf":
            logger.info("Rendering PDF through built-in pdf2image path for %s", self.file_name)
            return self._render_pdf_to_images(content), "pdf2image"
        if file_type in CONVERTER_TYPES:
            return self._convert_to_images(content), "converter"

        raise ValueError(f"Unsupported DeepParser file type: {self.file_type or file_type}")

    def _image_to_png(self, content: bytes) -> UploadPart:
        from PIL import Image, ImageOps

        with Image.open(BytesIO(content)) as image:
            image = ImageOps.exif_transpose(image)
            if image.mode in ("RGBA", "LA") or (image.mode == "P" and "transparency" in image.info):
                image = image.convert("RGBA")
                background = Image.new("RGBA", image.size, (255, 255, 255, 255))
                background.alpha_composite(image)
                image = background.convert("RGB")
            elif image.mode != "RGB":
                image = image.convert("RGB")

            buffer = BytesIO()
            image.save(buffer, format="PNG")

        return UploadPart(
            filename=f"{_safe_stem(self.file_name or 'image')}.png",
            content_type="image/png",
            content=buffer.getvalue(),
        )

    def _convert_to_images(self, content: bytes) -> list[UploadPart]:
        query = urllib.parse.urlencode(
            {
                "conversion_type": "PNG",
                "width": str(self.width),
                "height": str(self.height),
            }
        )
        url = self.converter_endpoint
        if "?" not in url:
            url = f"{url}?{query}"
        body, content_type = _multipart_body(
            {},
            [
                (
                    "file",
                    self.file_name or "document.bin",
                    mimetypes.guess_type(self.file_name)[0] or "application/octet-stream",
                    content,
                )
            ],
        )
        status_code, headers, response_body = _post_raw(url, body, content_type, self.timeout)
        if status_code < 200 or status_code >= 300:
            raise RuntimeError(f"converter HTTP {status_code}: {response_body[:500].decode('utf-8', errors='replace')}")
        return _converter_upload_parts(response_body, headers.get("Content-Type", ""))

    def _render_pdf_to_images(self, content: bytes) -> list[UploadPart]:
        import pypdfium2 as pdfium

        upload_parts: list[UploadPart] = []
        with parser_worker_limit("pdf_render", CONFIG.pdf_render_max_workers):
            pdf = pdfium.PdfDocument(content)
            try:
                scale = max(1, CONFIG.pdf_render_dpi) / 72
                for index in range(len(pdf)):
                    page = pdf[index]
                    bitmap = None
                    try:
                        bitmap = page.render(scale=scale)
                        image = bitmap.to_pil()
                        if image.mode != "RGB":
                            image = image.convert("RGB")
                        buffer = BytesIO()
                        image.save(buffer, format="PNG")
                        upload_parts.append(
                            UploadPart(
                                filename=f"{_safe_stem(self.file_name or 'document')}-p{index + 1:04d}.png",
                                content_type="image/png",
                                content=buffer.getvalue(),
                            )
                        )
                    finally:
                        _close_resource(bitmap)
                        _close_resource(page)
            finally:
                _close_resource(pdf)
        if not upload_parts:
            raise RuntimeError("PDF renderer produced no pages")
        return upload_parts

    def _call_deep_parser(self, upload_parts: list[UploadPart]) -> dict[str, Any]:
        fields = {
            "mode": self.mode,
            "page_numbers": ",".join(str(page) for page in range(1, len(upload_parts) + 1)),
            "rotate": "false",
            "debug": "true",
            "output_markdown": "true",
            "output_chunks": "true",
            "output_embed_base64": "false",
            "allow_partial_success": "false",
            "doc_name": self.file_name or "document",
            "original_page_num": str(len(upload_parts)),
        }
        body, content_type = _multipart_body(
            fields,
            [("files", part.filename, part.content_type, part.content) for part in upload_parts],
        )
        status_code, _, response_body = _post_raw(self.parser_endpoint, body, content_type, self.timeout)
        if status_code < 200 or status_code >= 300:
            raise RuntimeError(f"DeepParser HTTP {status_code}: {response_body[:500].decode('utf-8', errors='replace')}")
        payload = json.loads(response_body.decode("utf-8"))
        if payload.get("success") is False:
            raise RuntimeError(f"DeepParser returned success=false: {payload.get('status') or payload.get('message')}")
        return payload

    def _document_from_deep_parser_response(
        self,
        payload: dict[str, Any],
        upload_parts: list[UploadPart],
        image_source: str,
    ) -> Document:
        builder = _MarkdownGroundingBuilder(self.file_name or "document")
        element_count = 0

        for page_payload in payload.get("pages", []):
            page_number = int(page_payload.get("page_number") or page_payload.get("page") or 0)
            if page_number <= 0:
                continue
            builder.start_page(page_number)
            for index, raw_element in enumerate(page_payload.get("elements", []), start=1):
                text = _element_text(raw_element)
                if not text:
                    continue
                element_count += 1
                builder.append_element(page_number, index, raw_element, text)

        if element_count == 0:
            for page in payload.get("markdown_pages", []):
                page_number = int(page.get("page_number") or page.get("page") or 0)
                content = str(page.get("content") or "").strip()
                if page_number <= 0 or not content:
                    continue
                builder.start_page(page_number)
                element_count += 1
                builder.append_element(page_number, element_count, {"type": "MarkdownPage"}, content)

        markdown = builder.markdown()
        if not markdown.strip():
            raise RuntimeError("DeepParser response produced no text elements.")

        metadata = {
            "docreader_adapter": "weknora-docreader-deep-parser/v1",
            "parser_engine": DEEP_PARSER_ENGINE,
            "deep_parser_provider": DEEP_PARSER_ENGINE,
            "deep_parser_endpoint": self.parser_endpoint,
            "deep_parser_mode": self.mode,
            "deep_parser_image_source": image_source,
            "page_count": str(payload.get("total_pages") or len(upload_parts)),
            "element_count": str(element_count),
            "grounding_map": json.dumps(builder.groundings, ensure_ascii=False, sort_keys=True),
        }
        page_images, images = _deep_office_page_images(upload_parts)
        if page_images:
            metadata["deep_office_page_images"] = json.dumps(page_images, ensure_ascii=False, sort_keys=True)
        if image_source == "converter":
            metadata["converter_endpoint"] = self.converter_endpoint
        return Document(content=markdown, images=images, metadata=metadata)


class _MarkdownGroundingBuilder:
    def __init__(self, title: str) -> None:
        self.parts: list[str] = [f"# {title}\n\n"]
        self.cursor = len(self.parts[0])
        self.seen_pages: set[int] = set()
        self.groundings: list[dict[str, Any]] = []

    def start_page(self, page_number: int) -> None:
        if page_number in self.seen_pages:
            return
        self.seen_pages.add(page_number)
        self._append(f"## Page {page_number}\n\n")

    def append_element(self, page_number: int, index: int, raw_element: dict[str, Any], text: str) -> None:
        text = text.strip()
        start = self.cursor
        self._append(text + "\n\n")
        end = self.cursor
        element_type = str(raw_element.get("category") or raw_element.get("type") or "Text")
        element_id = str(raw_element.get("id") or f"p{page_number}-e{index}")
        self.groundings.append(
            {
                "element_id": element_id,
                "page": page_number,
                "element_type": element_type,
                "bbox": _bbox_values(raw_element.get("bbox")),
                "layout_order": raw_element.get("layout_order"),
                "char_start": start,
                "char_end": end,
                "text_hash": "sha256:" + hashlib.sha256(text.encode("utf-8")).hexdigest(),
            }
        )

    def markdown(self) -> str:
        return "".join(self.parts)

    def _append(self, value: str) -> None:
        self.parts.append(value)
        self.cursor += len(value)


def check_deep_parser_available(overrides: dict[str, str] | None = None) -> tuple[bool, str]:
    overrides = overrides or {}
    converter = _endpoint_url(
        overrides.get("deep_parser_converter_endpoint")
        or overrides.get("converter_endpoint")
        or os.environ.get("DEEP_PARSER_CONVERTER_ENDPOINT")
        or DEFAULT_CONVERTER_ENDPOINT
    )
    parser = _endpoint_url(
        overrides.get("deep_parser_endpoint")
        or overrides.get("parser_endpoint")
        or os.environ.get("DEEP_PARSER_ENDPOINT")
        or DEFAULT_PARSER_ENDPOINT
    )
    try:
        _get_json(_service_health_url(converter), timeout=3)
    except Exception as exc:
        return False, f"Converter unavailable: {exc}"
    try:
        _get_json(_service_health_url(parser), timeout=3)
    except Exception as exc:
        return False, f"DeepParser unavailable: {exc}"
    return True, ""


def _converter_upload_parts(body: bytes, content_type: str) -> list[UploadPart]:
    lowered = content_type.lower()
    if lowered.startswith("image/"):
        return [UploadPart(filename="page-0001.png", content_type=content_type.split(";", 1)[0], content=body)]
    if "zip" in lowered or body.startswith(b"PK\x03\x04"):
        return _upload_parts_from_zip(body)

    try:
        payload = json.loads(body.decode("utf-8"))
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"unsupported converter response content-type={content_type!r}") from exc

    if isinstance(payload, dict) and payload.get("success") is False:
        raise RuntimeError(payload.get("message") or payload.get("error_detail") or "converter returned success=false")

    parts = list(_upload_parts_from_json(payload))
    if not parts:
        keys = ", ".join(sorted(payload.keys())) if isinstance(payload, dict) else type(payload).__name__
        raise RuntimeError(f"converter response did not contain image payloads ({keys})")
    return parts


def _upload_parts_from_zip(body: bytes) -> list[UploadPart]:
    parts: list[UploadPart] = []
    with zipfile.ZipFile(BytesIO(body)) as archive:
        for name in sorted(archive.namelist()):
            if name.endswith("/"):
                continue
            ext = name.rsplit(".", 1)[-1].lower()
            if ext not in DIRECT_IMAGE_TYPES:
                continue
            content = archive.read(name)
            parts.append(UploadPart(filename=os.path.basename(name), content_type=mimetypes.guess_type(name)[0] or "image/png", content=content))
    if not parts:
        raise RuntimeError("converter zip did not contain image files")
    return parts


def _upload_parts_from_json(value: Any) -> list[UploadPart]:
    parts: list[UploadPart] = []
    for item in _walk_json(value):
        if not isinstance(item, dict):
            continue
        filename = str(item.get("filename") or item.get("file_name") or item.get("name") or f"page-{len(parts) + 1:04d}.png")
        content_type = str(item.get("content_type") or item.get("mime_type") or mimetypes.guess_type(filename)[0] or "image/png")
        data = item.get("base64") or item.get("image_base64") or item.get("content") or item.get("data") or item.get("file_content")
        if isinstance(data, str):
            decoded = _decode_base64_payload(data)
            if decoded:
                parts.append(UploadPart(filename=filename, content_type=content_type, content=decoded))
                continue
        url = item.get("url") or item.get("download_url") or item.get("file_url") or item.get("result_url")
        if isinstance(url, str) and url.startswith(("http://", "https://")):
            downloaded, downloaded_type = _download(url)
            parts.append(UploadPart(filename=filename, content_type=downloaded_type or content_type, content=downloaded))
    return parts


def _walk_json(value: Any) -> list[Any]:
    out = [value]
    if isinstance(value, dict):
        for nested in value.values():
            out.extend(_walk_json(nested))
    elif isinstance(value, list):
        for nested in value:
            out.extend(_walk_json(nested))
    return out


def _decode_base64_payload(value: str) -> bytes | None:
    if value.startswith("data:"):
        value = value.split(",", 1)[-1]
    try:
        decoded = base64.b64decode(value, validate=True)
    except Exception:
        return None
    return decoded or None


def _post_raw(url: str, body: bytes, content_type: str, timeout: float) -> tuple[int, dict[str, str], bytes]:
    request = urllib.request.Request(
        url,
        data=body,
        headers={
            "Accept": "*/*",
            "Content-Type": content_type,
            "Content-Length": str(len(body)),
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            return int(response.status), dict(response.headers), response.read()
    except urllib.error.HTTPError as exc:
        return int(exc.code), dict(exc.headers), exc.read()


def _multipart_body(fields: dict[str, str], files: list[tuple[str, str, str, bytes]]) -> tuple[bytes, str]:
    boundary = f"weknora-deep-parser-{uuid.uuid4().hex}"
    chunks: list[bytes] = []
    for name, value in fields.items():
        chunks.extend(
            [
                f"--{boundary}\r\n".encode(),
                f'Content-Disposition: form-data; name="{name}"\r\n\r\n'.encode(),
                str(value).encode(),
                b"\r\n",
            ]
        )
    for field_name, filename, content_type, content in files:
        chunks.extend(
            [
                f"--{boundary}\r\n".encode(),
                f'Content-Disposition: form-data; name="{field_name}"; filename="{filename}"\r\n'.encode(),
                f"Content-Type: {content_type}\r\n\r\n".encode(),
                content,
                b"\r\n",
            ]
        )
    chunks.append(f"--{boundary}--\r\n".encode())
    return b"".join(chunks), f"multipart/form-data; boundary={boundary}"


def _get_json(url: str, timeout: float) -> dict[str, Any]:
    with urllib.request.urlopen(url, timeout=timeout) as response:
        return json.loads(response.read().decode("utf-8"))


def _download(url: str) -> tuple[bytes, str]:
    with urllib.request.urlopen(url, timeout=60) as response:
        return response.read(), response.headers.get("Content-Type", "")


def _close_resource(resource: Any) -> None:
    close = getattr(resource, "close", None)
    if close:
        close()


def _safe_stem(value: str) -> str:
    stem = os.path.splitext(os.path.basename(value))[0]
    safe = "".join(char if char.isascii() and (char.isalnum() or char in "._-") else "-" for char in stem)
    return safe.strip("-") or "document"


def _endpoint_url(endpoint: str) -> str:
    endpoint = endpoint.strip()
    if endpoint.startswith(("http://", "https://")):
        return endpoint
    return f"http://{endpoint}"


def _service_health_url(endpoint: str) -> str:
    parsed = urllib.parse.urlparse(endpoint)
    if not parsed.scheme or not parsed.netloc:
        return endpoint.rstrip("/") + "/health"
    return urllib.parse.urlunparse((parsed.scheme, parsed.netloc, "/health", "", "", ""))


def _element_text(element: dict[str, Any]) -> str:
    for key in ("content", "text", "markdown", "html", "caption"):
        value = element.get(key)
        if isinstance(value, str) and value.strip():
            return value.strip()
    return ""


def _bbox_values(value: Any) -> list[float]:
    if isinstance(value, dict):
        return [float(value[key]) for key in ("x1", "y1", "x2", "y2") if key in value]
    if isinstance(value, list):
        return [float(item) for item in value]
    return []


def _deep_office_page_images(upload_parts: list[UploadPart]) -> tuple[list[dict[str, Any]], dict[str, str]]:
    records: list[dict[str, Any]] = []
    images: dict[str, str] = {}
    for index, part in enumerate(upload_parts, start=1):
        original_ref = f"{DEEP_OFFICE_PAGE_IMAGE_REF_PREFIX}{index}/{part.filename}"
        record: dict[str, Any] = {
            "page": index,
            "original_ref": original_ref,
            "filename": part.filename,
            "mime_type": part.content_type or "image/png",
            "sha256": "sha256:" + hashlib.sha256(part.content).hexdigest(),
            "byte_size": len(part.content),
        }
        width, height = _image_dimensions(part.content)
        if width and height:
            record["width"] = width
            record["height"] = height
        records.append(record)
        images[original_ref] = base64.b64encode(part.content).decode("ascii")
    return records, images


def _image_dimensions(content: bytes) -> tuple[int, int]:
    try:
        from PIL import Image

        with Image.open(BytesIO(content)) as image:
            return int(image.width), int(image.height)
    except Exception:
        return 0, 0
