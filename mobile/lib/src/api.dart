/// Galeriku mobile API client.
///
/// Mirrors the Go backend's standard JSON envelope and error codes.
/// Shares the same contract surface as web/src/types/api.ts.
library;

import 'dart:convert';
import 'package:http/http.dart' as http;

class ApiError implements Exception {
  final int status;
  final String code;
  final String message;
  final String? requestId;

  ApiError(this.status, this.code, this.message, {this.requestId});

  @override
  String toString() => 'ApiError($status $code): $message';
}

class GalerikuApi {
  final String baseUrl;
  final http.Client _client;
  String? _accessToken;

  GalerikuApi({required this.baseUrl, http.Client? client})
      : _client = client ?? http.Client();

  void setAccessToken(String? token) => _accessToken = token;

  Map<String, String> _headers({String? idempotencyKey}) => {
        'Content-Type': 'application/json',
        if (_accessToken != null) 'Authorization': 'Bearer $_accessToken',
        if (idempotencyKey != null) 'Idempotency-Key': idempotencyKey,
      };

  Future<T> _request<T>(String method, String path,
      {Map<String, dynamic>? body, String? idempotencyKey}) async {
    final uri = Uri.parse('$baseUrl$path');
    late http.Response response;
    switch (method) {
      case 'GET':
      case 'DELETE':
        response = await _client.send(http.Request(method, uri)..headers.addAll(_headers(idempotencyKey: idempotencyKey)))
            .then(http.Response.fromStream);
        break;
      case 'PUT':
        response = await _client.put(uri,
            headers: _headers(idempotencyKey: idempotencyKey),
            body: body != null ? jsonEncode(body) : null);
        break;
      default: // POST
        response = await _client.post(uri,
            headers: _headers(idempotencyKey: idempotencyKey),
            body: body != null ? jsonEncode(body) : null);
    }

    final payload = jsonDecode(response.body) as Map<String, dynamic>;
    if (payload['success'] == true) {
      return payload['data'] as T;
    }
    final error = payload['error'] as Map<String, dynamic>;
    throw ApiError(response.statusCode, error['code'] as String, error['message'] as String,
        requestId: error['request_id'] as String?);
  }

  // Auth

  Future<Map<String, dynamic>> login(String identifier, String password) =>
      _request('POST', '/api/v1/auth/login', body: {'identifier': identifier, 'password': password});

  Future<Map<String, dynamic>> register(String email, String password, String displayName) =>
      _request('POST', '/api/v1/auth/register', body: {'email': email, 'password': password, 'display_name': displayName});

  Future<Map<String, dynamic>> me() => _request('GET', '/api/v1/auth/me');

  // Discovery (public BIB search)

  Future<Map<String, dynamic>> searchBIB(String bibNumber, [String? eventId]) =>
      _request('POST', '/api/v1/search/bib', body: {'bib_number': bibNumber, if (eventId != null) 'event_id': eventId});

  // Cart and orders

  Future<Map<String, dynamic>> getCart() => _request('GET', '/api/v1/cart');

  Future<Map<String, dynamic>> addToCart(String photoId) =>
      _request('POST', '/api/v1/cart/items', body: {'photo_id': photoId});

  Future<Map<String, dynamic>> removeFromCart(String cartItemId) =>
      _request('DELETE', '/api/v1/cart/items/$cartItemId');

  Future<Map<String, dynamic>> createOrder(List<String> photoIds, String idempotencyKey) =>
      _request('POST', '/api/v1/orders', body: {'photo_ids': photoIds}, idempotencyKey: idempotencyKey);

  // Payments and downloads

  Future<Map<String, dynamic>> initiatePayment(String orderId, String method) =>
      _request('POST', '/api/v1/payments/initiate', body: {'order_id': orderId, 'payment_method': method});

  Future<Map<String, dynamic>> grantDownload(String orderItemId, String assetType) =>
      _request('POST', '/api/v1/downloads/grant/$orderItemId', body: {'requested_asset': assetType});

  // Host application

  Future<Map<String, dynamic>> applyAsHost(Map<String, String> input) =>
      _request('POST', '/api/v1/hosts/apply', body: input);

  // Events (FotoTree)

  Future<Map<String, dynamic>> listEvents() => _request('GET', '/api/v1/events');
}
