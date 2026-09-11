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
    const controller = new AbortController();
    let timedOut = false;
    const abortFromCaller = () => controller.abort(signal?.reason || new DOMException("Request cancelled", "AbortError"));
    if (signal?.aborted) abortFromCaller();
    else signal?.addEventListener("abort", abortFromCaller, { once: true });
    const timer = setTimeout(() => {
      timedOut = true;
      controller.abort(new DOMException("Request timed out", "TimeoutError"));
    }, Math.max(1, Number(timeout) || 12000));
    try {
      const response = await fetch(path, {
        credentials: "same-origin",
        headers: headers(csrf, extraHeaders),
        ...fetchOptions,
        signal: controller.signal,
      });
      const data = await decode(response);
      if (!response.ok) {
        const error = new Error(data.error || response.statusText || `HTTP ${response.status}`);
        error.data = data;
        error.status = response.status;
        error.retryAfter = Number(response.headers.get("Retry-After") || data.retry_after_seconds || 0);
        error.code = data.code || "http_error";
        throw error;
      }
      return data;
    } catch (error) {
      if (error?.name === "AbortError" || error?.name === "TimeoutError") {
        const aborted = new Error(timedOut ? "Request timed out" : "Request cancelled");
        aborted.name = error.name;
        aborted.code = timedOut ? "request_timeout" : "request_cancelled";
        aborted.cause = error;
        throw aborted;
      }
      if (!error.code) error.code = "network_error";
      throw error;
    } finally {
      clearTimeout(timer);
      signal?.removeEventListener("abort", abortFromCaller);
    }
  }

  async function streamJSONLines(path, body, options = {}) {
    const controller = new AbortController();
    let timedOut = false;
    const abortFromCaller = () => controller.abort(options.signal?.reason || new DOMException("Request cancelled", "AbortError"));
    if (options.signal?.aborted) abortFromCaller();
    else options.signal?.addEventListener("abort", abortFromCaller, { once: true });
    const timer = setTimeout(() => {
      timedOut = true;
      controller.abort(new DOMException("Stream timed out", "TimeoutError"));
    }, Math.max(1, Number(options.timeout) || 30000));
    let response;
    try {
      response = await fetch(path, {
        method: "POST",
        credentials: "same-origin",
        headers: headers(options.csrf),
        body: JSON.stringify(body),
        signal: controller.signal,
      });
    } catch (error) {
      clearTimeout(timer);
      options.signal?.removeEventListener("abort", abortFromCaller);
      const wrapped = new Error(timedOut ? "Request timed out" : (controller.signal.aborted ? "Request cancelled" : error.message));
      wrapped.code = timedOut ? "request_timeout" : (controller.signal.aborted ? "request_cancelled" : "network_error");
      throw wrapped;
    }
    try {
      if (!response.ok) {
        const data = await decode(response);
        const error = new Error(data.error || response.statusText || `HTTP ${response.status}`);
        error.data = data;
        error.status = response.status;
        error.code = data.code || "http_error";
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
    } catch (error) {
      if (controller.signal.aborted) {
        const wrapped = new Error(timedOut ? "Request timed out" : "Request cancelled");
        wrapped.code = timedOut ? "request_timeout" : "request_cancelled";
        throw wrapped;
      }
      throw error;
    } finally {
      clearTimeout(timer);
      options.signal?.removeEventListener("abort", abortFromCaller);
    }
  }

  return { request, streamJSONLines };
})();
