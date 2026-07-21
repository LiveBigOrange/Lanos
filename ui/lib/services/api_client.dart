import 'dart:convert';
import 'dart:io';

/// Thin HTTP client wrapping the gcd local REST API.
///
/// All requests carry `Authorization: Bearer <token>` (see PRD §5.1.5).
/// Bearer auth is skipped for `/api/v1/ping`.
class ApiClient {
  ApiClient({required this.port, required this.token, required this.version});

  final int port;
  final String token;
  final String version;

  HttpClient? _http;

  HttpClient get http => _http ??= HttpClient();

  Uri _uri(String path) {
    return Uri.http('127.0.0.1:$port', path);
  }

  Future<Map<String, dynamic>> get(String path) async {
    final req = await http.getUrl(_uri(path));
    _attachAuth(req, path);
    final resp = await req.close();
    return _decode(resp);
  }

  /// Like [get] but returns the raw response body as a String.
  /// Useful for endpoints that return non-JSON content (e.g. CSV export).
  Future<String> getRaw(String path) async {
    final req = await http.getUrl(_uri(path));
    _attachAuth(req, path);
    final resp = await req.close();
    final body = await resp.transform(utf8.decoder).join();
    if (resp.statusCode < 200 || resp.statusCode >= 300) {
      throw ApiException(resp.statusCode, body);
    }
    return body;
  }

  Future<Map<String, dynamic>> post(
    String path, [
    Map<String, dynamic>? body,
  ]) async {
    final req = await http.postUrl(_uri(path));
    _attachAuth(req, path);
    if (body != null) {
      req.headers.contentType = ContentType.json;
      req.add(utf8.encode(jsonEncode(body)));
    }
    final resp = await req.close();
    return _decode(resp);
  }

  Future<Map<String, dynamic>> delete(String path) async {
    final req = await http.deleteUrl(_uri(path));
    _attachAuth(req, path);
    final resp = await req.close();
    return _decode(resp);
  }

  void _attachAuth(HttpClientRequest req, String path) {
    // /api/v1/ping is exempt per PRD §5.1.5.
    if (path == '/api/v1/ping') return;
    req.headers.set('Authorization', 'Bearer $token');
  }

  Future<Map<String, dynamic>> _decode(HttpClientResponse resp) async {
    final body = await resp.transform(utf8.decoder).join();
    if (resp.statusCode < 200 || resp.statusCode >= 300) {
      throw ApiException(resp.statusCode, body);
    }
    if (body.isEmpty) return {};
    return jsonDecode(body) as Map<String, dynamic>;
  }

  /// Convenience: health-check endpoint.
  Future<bool> ping() async {
    try {
      final r = await get('/api/v1/ping');
      return r['ok'] == true;
    } catch (_) {
      return false;
    }
  }

  void close() {
    _http?.close();
    _http = null;
  }
}

class ApiException implements Exception {
  ApiException(this.statusCode, this.body);

  final int statusCode;
  final String body;

  @override
  String toString() => 'ApiClient: HTTP $statusCode: $body';
}
