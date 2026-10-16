import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:galeriku_mobile/src/api.dart';
import 'package:galeriku_mobile/src/models.dart';
import 'package:http/http.dart' as http;

/// A canned-response http client standing in for the Go backend.
class FakeApi extends http.BaseClient {
  final List<http.Response> _queue = [];

  void enqueue(int status, Map<String, dynamic> body) {
    _queue.add(http.Response(jsonEncode(body), status, headers: {'content-type': 'application/json'}));
  }

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    final response = _queue.removeAt(0);
    return http.StreamedResponse(
      Stream.value(response.bodyBytes),
      response.statusCode,
      headers: response.headers,
    );
  }
}

void main() {
  late FakeApi transport;
  late GalerikuApi api;

  setUp(() {
    transport = FakeApi();
    api = GalerikuApi(baseUrl: 'http://test', client: transport);
  });

  group('GalerikuApi contract surface', () {
    test('login returns access_token from success envelope', () async {
      transport.enqueue(200, {'success': true, 'data': {'access_token': 'tok123'}, 'meta': {'request_id': 'r'}});
      final data = await api.login('user@galeriku.com', 'pass');
      expect(data['access_token'], 'tok123');
    });

    test('searchBIB parses bib_normalized and photos', () async {
      transport.enqueue(200, {
        'success': true,
        'data': {
          'bib_normalized': '10452',
          'total_photos': 1,
          'photos': [
            {'photo_id': 'p1', 'price_idr': 45000, 'bib_checkout_requires_verification': true}
          ]
        },
        'meta': {'request_id': 'r'}
      });
      final data = await api.searchBIB('10452');
      final result = BibSearchResult.fromJson(data);
      expect(result.bibNormalized, '10452');
      expect(result.totalPhotos, 1);
      expect(result.photos.first.bibCheckoutRequiresVerification, true);
    });

    test('getCart parses cart items', () async {
      transport.enqueue(200, {
        'success': true,
        'data': {
          'cart_id': 'c1',
          'total_items': 2,
          'total_amount_idr': 90000,
          'items': [
            {'cart_item_id': 'ci1', 'photo_id': 'p1', 'price_idr': 45000},
            {'cart_item_id': 'ci2', 'photo_id': 'p2', 'price_idr': 45000},
          ]
        },
        'meta': {'request_id': 'r'}
      });
      final data = await api.getCart();
      final cart = Cart.fromJson(data);
      expect(cart.totalItems, 2);
      expect(cart.totalAmountIdr, 90000);
      expect(cart.items.length, 2);
    });

    test('ApiError is thrown for error envelopes with the backend code', () async {
      transport.enqueue(422, {
        'success': false,
        'error': {'code': 'QUOTA_EXCEEDED', 'message': 'insufficient quota', 'request_id': 'r1'},
        'meta': {'timestamp': 't', 'request_id': 'r1'}
      });
      expect(
        () => api.searchBIB('999'),
        throwsA(isA<ApiError>().having((e) => e.code, 'code', 'QUOTA_EXCEEDED')),
      );
    });

    test('payment initiation parses QRIS fields', () async {
      transport.enqueue(200, {
        'success': true,
        'data': {
          'payment_id': 'pay1',
          'provider': 'MIDTRANS',
          'qris_string': '00020101021226STUB',
          'amount_idr': 90000,
        },
        'meta': {'request_id': 'r'}
      });
      final data = await api.initiatePayment('order1', 'QRIS');
      final payment = PaymentInitiation.fromJson(data);
      expect(payment.provider, 'MIDTRANS');
      expect(payment.amountIdr, 90000);
      expect(payment.qrisString, contains('000201'));
    });
  });
}
