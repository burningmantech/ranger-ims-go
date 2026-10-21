//
// See the file COPYRIGHT for copyright information.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

//! Content sniffing for uploaded attachments, standing in for the Go server's
//! gabriel-vasile/mimetype. Binary formats come from the `infer` crate; text
//! formats are recognized here, since those are what matter for keeping
//! scriptable content (HTML, SVG) from being previewed.

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Mime {
    /// The media type with any parameters, e.g. "text/plain; charset=utf-8".
    pub mime: String,
    /// The usual file extension, with its dot, or "" if there's none.
    pub extension: String,
}

impl Mime {
    fn new(mime: &str, extension: &str) -> Mime {
        Mime {
            mime: mime.to_string(),
            extension: extension.to_string(),
        }
    }
}

/// mimetype reads at most this much of a file.
const READ_LIMIT: usize = 3072;

pub fn detect(data: &[u8]) -> Mime {
    let head = &data[..data.len().min(READ_LIMIT)];
    // infer knows some text formats too, but reports them without a charset;
    // those fall through to the text detection below.
    if let Some(t) = infer::get(head)
        .filter(|t| !t.mime_type().starts_with("text/") && !t.mime_type().contains("xml"))
    {
        let mime = match t.mime_type() {
            "image/heif" => "image/heic",
            "video/quicktime" => "video/quicktime",
            other => other,
        };
        return Mime::new(mime, &format!(".{}", t.extension()));
    }
    if !is_text(head) {
        return Mime::new("application/octet-stream", "");
    }
    let trimmed = trim_leading_space(head);
    if is_html(trimmed) {
        return Mime::new("text/html; charset=utf-8", ".html");
    }
    let lower = String::from_utf8_lossy(head).to_ascii_lowercase();
    if lower.contains("<svg") {
        return Mime::new("image/svg+xml", ".svg");
    }
    if trimmed.starts_with(b"<?xml") {
        return Mime::new("text/xml; charset=utf-8", ".xml");
    }
    if (trimmed.starts_with(b"{") || trimmed.starts_with(b"["))
        && serde_json::from_slice::<serde_json::Value>(data).is_ok()
    {
        return Mime::new("application/json", ".json");
    }
    Mime::new("text/plain; charset=utf-8", ".txt")
}

fn trim_leading_space(b: &[u8]) -> &[u8] {
    let start = b
        .iter()
        .position(|c| !c.is_ascii_whitespace())
        .unwrap_or(b.len());
    &b[start..]
}

/// Text, as mimetype judges it: no bytes that only binary files contain.
fn is_text(b: &[u8]) -> bool {
    if b.is_empty() {
        return true;
    }
    let valid_prefix = match std::str::from_utf8(b) {
        Ok(_) => true,
        // A multi-byte character cut off by the read limit is still text.
        Err(e) => e.error_len().is_none(),
    };
    valid_prefix
        && !b
            .iter()
            .any(|&c| c < 0x20 && !matches!(c, b'\t' | b'\n' | b'\r' | 0x0c | 0x1b))
}

fn is_html(b: &[u8]) -> bool {
    const SIGS: [&[u8]; 17] = [
        b"<!DOCTYPE HTML",
        b"<HTML",
        b"<HEAD",
        b"<SCRIPT",
        b"<IFRAME",
        b"<H1",
        b"<DIV",
        b"<FONT",
        b"<TABLE",
        b"<A",
        b"<STYLE",
        b"<TITLE",
        b"<B",
        b"<BODY",
        b"<BR",
        b"<P",
        b"<!--",
    ];
    SIGS.iter().any(|sig| {
        b.len() > sig.len()
            && b[..sig.len()].eq_ignore_ascii_case(sig)
            && matches!(b[sig.len()], b' ' | b'>')
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn detects_text() {
        assert_eq!(
            detect(b"Alice's own report attachment").mime,
            "text/plain; charset=utf-8"
        );
        assert_eq!(detect(b"Alice's own report attachment").extension, ".txt");
        assert_eq!(detect(b"").mime, "text/plain; charset=utf-8");
    }

    #[test]
    fn detects_html_and_svg() {
        assert_eq!(
            detect(b"<html><body><script>alert(1)</script></body></html>").mime,
            "text/html; charset=utf-8"
        );
        assert_eq!(
            detect(br#"<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>"#).mime,
            "image/svg+xml"
        );
    }

    #[test]
    fn detects_png() {
        let png = [
            0x89, b'P', b'N', b'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0x0d, b'I', b'H', b'D', b'R',
        ];
        let m = detect(&png);
        assert_eq!(m.mime, "image/png");
        assert_eq!(m.extension, ".png");
    }

    #[test]
    fn detects_binary() {
        assert_eq!(
            detect(&[0u8, 1, 2, 3, 0xff]).mime,
            "application/octet-stream"
        );
    }
}
