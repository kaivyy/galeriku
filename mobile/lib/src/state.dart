/// App-wide state: auth token, cart snapshot, and API client.
library;

import 'package:flutter/material.dart';
import 'api.dart';
import 'models.dart';

class AppState extends ChangeNotifier {
  final GalerikuApi api;
  String? _token;
  Cart? _cart;
  bool _loading = false;
  String? _error;

  AppState({required this.api});

  String? get token => _token;
  Cart? get cart => _cart;
  bool get loading => _loading;
  String? get error => _error;

  void _setLoading(bool v) { _loading = v; notifyListeners(); }
  void _setError(String? e) { _error = e; notifyListeners(); }

  // Auth

  Future<bool> login(String identifier, String password) async {
    _setLoading(true);
    try {
      final data = await api.login(identifier, password);
      _token = data['access_token'] as String;
      api.setAccessToken(_token);
      _setError(null);
      return true;
    } on ApiError catch (e) {
      _setError(e.message);
      return false;
    } finally {
      _setLoading(false);
    }
  }

  Future<bool> register(String email, String password, String displayName) async {
    _setLoading(true);
    try {
      await api.register(email, password, displayName);
      _setError(null);
      return true;
    } on ApiError catch (e) {
      _setError(e.message);
      return false;
    } finally {
      _setLoading(false);
    }
  }

  void logout() {
    _token = null;
    api.setAccessToken(null);
    _cart = null;
    notifyListeners();
  }

  // Cart

  Future<void> refreshCart() async {
    try {
      final data = await api.getCart();
      _cart = Cart.fromJson(data);
      notifyListeners();
    } on ApiError catch (e) {
      _setError(e.message);
    }
  }

  Future<bool> addToCart(String photoId) async {
    try {
      await api.addToCart(photoId);
      await refreshCart();
      return true;
    } on ApiError catch (e) {
      _setError(e.message);
      return false;
    }
  }

  Future<bool> removeFromCart(String cartItemId) async {
    try {
      await api.removeFromCart(cartItemId);
      await refreshCart();
      return true;
    } on ApiError catch (e) {
      _setError(e.message);
      return false;
    }
  }

  Future<OrderResult?> createOrder(List<String> photoIds) async {
    try {
      final key = DateTime.now().microsecondsSinceEpoch.toString();
      final data = await api.createOrder(photoIds, key);
      return OrderResult.fromJson(data);
    } on ApiError catch (e) {
      _setError(e.message);
      return null;
    }
  }

  Future<PaymentInitiation?> initiatePayment(String orderId, String method) async {
    try {
      final data = await api.initiatePayment(orderId, method);
      return PaymentInitiation.fromJson(data);
    } on ApiError catch (e) {
      _setError(e.message);
      return null;
    }
  }
}
