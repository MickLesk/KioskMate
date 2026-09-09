"use strict";

window.KioskMateAPI = (() => {
  function headers(csrf, extra = {}) {
    return {
      "Content-Type": "application/json",
      ...(csrf ? { "X-KioskMate-CSRF": csrf } : {}),
      ...extra,
    };
  }

  async function decode(response) {
    const text = await response.text();
    if (!text) return {};
    try {
      return JSON.parse(text);
    } catch {
      return { error: text };
    }
  }

  async function request(path, options = {}) {
    const { timeout = 12000, signal, csrf, headers: extraHeaders, ...fetchOptions } = options;
    const controller = signal ? null : new AbortController();
    const timer = controller ? setTimeout(() => controller.abort(), Number(timeout)) : null;
    try {
      const response = await fetch(path, {
        credentials: "same-origin",
        headers: headers(csrf, extraHeaders),
        ...fetchOptions,
        signal: signal || controller?.signal,
      });
      const data = await decode(response);
      if (!response.ok) {
        const error = new Error(data.error || response.statusText || `HTTP ${response.status}`);
        error.data = data;
        error.status = response.status;
        error.retryAfter = Number(response.headers.get("Retry-After") || data.retry_after_seconds || 0);
        throw error;
      }
      return data;
    } finally {
      if (timer) clearTimeout(timer);
    }
  }

  async function streamJSONLines(path, body, options = {}) {
    const response = await fetch(path, {
      method: "POST",
      credentials: "same-origin",
      headers: headers(options.csrf),
      body: JSON.stringify(body),
      signal: options.signal,
    });
    if (!response.ok) {
      const data = await decode(response);
      const error = new Error(data.error || response.statusText || `HTTP ${response.status}`);
      error.data = data;
      error.status = response.status;
      throw error;
    }
    if (!response.body) throw new Error("Streaming response is not available");
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";
      for (const line of lines) {
        if (line.trim()) options.onEvent(JSON.parse(line));
      }
    }
    buffer += decoder.decode();
    if (buffer.trim()) options.onEvent(JSON.parse(buffer));
  }

  return { request, streamJSONLines };
})();
