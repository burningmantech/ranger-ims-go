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

//! Makes the 400s that hyper writes for requests it can't even parse look
//! like the ones Go's net/http writes, which clients (and the Go API
//! integration suite) rely on: a text/plain body, and a closed connection.

use std::io;
use std::pin::Pin;
use std::task::{Context, Poll};

use tokio::io::{AsyncRead, AsyncWrite, ReadBuf};

const HYPER_400_PREFIX: &[u8] = b"HTTP/1.1 400 Bad Request\r\n";
const GO_400: &[u8] =
    b"HTTP/1.1 400 Bad Request\r\nContent-Type: text/plain; charset=utf-8\r\nConnection: close\r\n\r\n400 Bad Request";

pub struct GoErrorIo<T> {
    inner: T,
    /// Replacement bytes still to be written in place of what hyper wrote.
    pending: Vec<u8>,
}

impl<T> GoErrorIo<T> {
    pub fn new(inner: T) -> Self {
        GoErrorIo {
            inner,
            pending: Vec::new(),
        }
    }
}

/// Whether `buf` is hyper's own parse-error response. Every 400 the handlers
/// write carries a Content-Type, so one without is hyper's.
fn is_hyper_parse_error(buf: &[u8]) -> bool {
    if !buf.starts_with(HYPER_400_PREFIX) {
        return false;
    }
    let head_end = buf
        .windows(4)
        .position(|w| w == b"\r\n\r\n")
        .unwrap_or(buf.len());
    !String::from_utf8_lossy(&buf[..head_end])
        .to_ascii_lowercase()
        .contains("\r\ncontent-type:")
}

impl<T: AsyncWrite + Unpin> GoErrorIo<T> {
    fn poll_drain(&mut self, cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        while !self.pending.is_empty() {
            let n = std::task::ready!(Pin::new(&mut self.inner).poll_write(cx, &self.pending))?;
            if n == 0 {
                return Poll::Ready(Err(io::ErrorKind::WriteZero.into()));
            }
            self.pending.drain(..n);
        }
        Poll::Ready(Ok(()))
    }
}

impl<T: AsyncRead + Unpin> AsyncRead for GoErrorIo<T> {
    fn poll_read(
        mut self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &mut ReadBuf<'_>,
    ) -> Poll<io::Result<()>> {
        Pin::new(&mut self.inner).poll_read(cx, buf)
    }
}

impl<T: AsyncWrite + Unpin> AsyncWrite for GoErrorIo<T> {
    fn poll_write(
        mut self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &[u8],
    ) -> Poll<io::Result<usize>> {
        std::task::ready!(self.poll_drain(cx))?;
        if is_hyper_parse_error(buf) {
            self.pending = GO_400.to_vec();
            // Claim the whole buffer as written; the replacement goes out instead.
            let _ = self.poll_drain(cx)?;
            return Poll::Ready(Ok(buf.len()));
        }
        Pin::new(&mut self.inner).poll_write(cx, buf)
    }

    fn poll_flush(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        std::task::ready!(self.poll_drain(cx))?;
        Pin::new(&mut self.inner).poll_flush(cx)
    }

    fn poll_shutdown(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        std::task::ready!(self.poll_drain(cx))?;
        Pin::new(&mut self.inner).poll_shutdown(cx)
    }

    // hyper then flattens its writes, so a whole response arrives in one buffer.
    fn is_write_vectored(&self) -> bool {
        false
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn recognizes_hyper_400() {
        assert!(is_hyper_parse_error(
            b"HTTP/1.1 400 Bad Request\r\ncontent-length: 0\r\ndate: x\r\n\r\n"
        ));
        assert!(!is_hyper_parse_error(
            b"HTTP/1.1 400 Bad Request\r\ncontent-type: application/problem+json\r\n\r\n{}"
        ));
        assert!(!is_hyper_parse_error(b"HTTP/1.1 200 OK\r\n\r\n"));
    }

    #[tokio::test]
    async fn rewrites_hyper_400() {
        use tokio::io::AsyncWriteExt;
        let mut out = GoErrorIo::new(Vec::<u8>::new());
        out.write_all(b"HTTP/1.1 400 Bad Request\r\ncontent-length: 0\r\n\r\n")
            .await
            .unwrap();
        out.flush().await.unwrap();
        assert_eq!(out.inner, GO_400);
    }
}
