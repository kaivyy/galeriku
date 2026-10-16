/// Domain models mirroring the backend contract types.
library;

class PhotoSearchHit {
  final String photoId;
  final int priceIdr;
  final bool bibCheckoutRequiresVerification;

  PhotoSearchHit({required this.photoId, required this.priceIdr, required this.bibCheckoutRequiresVerification});

  factory PhotoSearchHit.fromJson(Map<String, dynamic> j) => PhotoSearchHit(
        photoId: j['photo_id'] as String,
        priceIdr: (j['price_idr'] as num).toInt(),
        bibCheckoutRequiresVerification: j['bib_checkout_requires_verification'] as bool? ?? false,
      );
}

class BibSearchResult {
  final String bibNormalized;
  final int totalPhotos;
  final List<PhotoSearchHit> photos;

  BibSearchResult({required this.bibNormalized, required this.totalPhotos, required this.photos});

  factory BibSearchResult.fromJson(Map<String, dynamic> j) => BibSearchResult(
        bibNormalized: j['bib_normalized'] as String,
        totalPhotos: (j['total_photos'] as num).toInt(),
        photos: (j['photos'] as List<dynamic>? ?? [])
            .map((e) => PhotoSearchHit.fromJson(e as Map<String, dynamic>))
            .toList(),
      );
}

class CartItem {
  final String cartItemId;
  final String photoId;
  final int priceIdr;

  CartItem({required this.cartItemId, required this.photoId, required this.priceIdr});

  factory CartItem.fromJson(Map<String, dynamic> j) => CartItem(
        cartItemId: j['cart_item_id'] as String,
        photoId: j['photo_id'] as String,
        priceIdr: (j['price_idr'] as num).toInt(),
      );
}

class Cart {
  final String cartId;
  final int totalItems;
  final int totalAmountIdr;
  final List<CartItem> items;

  Cart({required this.cartId, required this.totalItems, required this.totalAmountIdr, required this.items});

  factory Cart.fromJson(Map<String, dynamic> j) => Cart(
        cartId: j['cart_id'] as String,
        totalItems: (j['total_items'] as num).toInt(),
        totalAmountIdr: (j['total_amount_idr'] as num).toInt(),
        items: (j['items'] as List<dynamic>? ?? [])
            .map((e) => CartItem.fromJson(e as Map<String, dynamic>))
            .toList(),
      );
}

class OrderResult {
  final String orderId;
  final String orderNumber;
  final int totalAmountIdr;
  final String status;

  OrderResult({required this.orderId, required this.orderNumber, required this.totalAmountIdr, required this.status});

  factory OrderResult.fromJson(Map<String, dynamic> j) => OrderResult(
        orderId: j['order_id'] as String,
        orderNumber: j['order_number'] as String,
        totalAmountIdr: (j['total_amount_idr'] as num).toInt(),
        status: j['status'] as String,
      );
}

class PaymentInitiation {
  final String paymentId;
  final String provider;
  final String? qrisString;
  final String? qrisImageUrl;
  final int amountIdr;

  PaymentInitiation({required this.paymentId, required this.provider, this.qrisString, this.qrisImageUrl, required this.amountIdr});

  factory PaymentInitiation.fromJson(Map<String, dynamic> j) => PaymentInitiation(
        paymentId: j['payment_id'] as String,
        provider: j['provider'] as String,
        qrisString: j['qris_string'] as String?,
        qrisImageUrl: j['qris_image_url'] as String?,
        amountIdr: (j['amount_idr'] as num).toInt(),
      );
}

class DownloadGrantView {
  final String downloadToken;
  final String assetType;
  final String? watermarkApplied;
  final String downloadUrl;

  DownloadGrantView({required this.downloadToken, required this.assetType, this.watermarkApplied, required this.downloadUrl});

  factory DownloadGrantView.fromJson(Map<String, dynamic> j) => DownloadGrantView(
        downloadToken: j['download_token'] as String,
        assetType: j['asset_type'] as String,
        watermarkApplied: j['watermark_applied'] as String?,
        downloadUrl: j['download_url'] as String,
      );
}
