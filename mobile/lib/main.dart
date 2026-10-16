/// Galeriku mobile app entry point.
///
/// Five-tab layout per the Phase 8.1 acceptance criteria: feed (GaleriGram),
/// AI photo search (selfie/BIB), cart, QRIS payment, and collections.
library;

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import 'src/api.dart';
import 'src/models.dart';
import 'src/state.dart';

void main() {
  final api = GalerikuApi(baseUrl: const String.fromEnvironment('API_BASE_URL', defaultValue: 'http://localhost:8080'));
  runApp(GalerikuApp(api: api));
}

class GalerikuApp extends StatelessWidget {
  final GalerikuApi api;

  const GalerikuApp({super.key, required this.api});

  @override
  Widget build(BuildContext context) {
    return ChangeNotifierProvider(
      create: (_) => AppState(api: api),
      child: MaterialApp(
        title: 'Galeriku',
        theme: ThemeData(colorSchemeSeed: const Color(0xFF1B5E20), useMaterial3: true),
        darkTheme: ThemeData(colorSchemeSeed: const Color(0xFF1B5E20), brightness: Brightness.dark, useMaterial3: true),
        home: const HomeShell(),
      ),
    );
  }
}

class HomeShell extends StatefulWidget {
  const HomeShell({super.key});

  @override
  State<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends State<HomeShell> {
  int _index = 0;

  static const _tabs = [
    FeedTab(),
    BibSearchTab(),
    CartTab(),
    CheckoutTab(),
    CollectionTab(),
  ];

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: _tabs[_index],
      bottomNavigationBar: NavigationBar(
        selectedIndex: _index,
        onDestinationSelected: (i) => setState(() => _index = i),
        destinations: const [
          NavigationDestination(icon: Icon(Icons.photo_library_outlined), label: 'Galeri'),
          NavigationDestination(icon: Icon(Icons.search), label: 'Cari'),
          NavigationDestination(icon: Icon(Icons.shopping_cart_outlined), label: 'Keranjang'),
          NavigationDestination(icon: Icon(Icons.qr_code), label: 'Bayar'),
          NavigationDestination(icon: Icon(Icons.collections_outlined), label: 'Koleksi'),
        ],
      ),
    );
  }
}

class FeedTab extends StatelessWidget {
  const FeedTab({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Galeriku')),
      body: const Center(child: Text('GaleriGram feed', style: TextStyle(fontSize: 18))),
    );
  }
}

class BibSearchTab extends StatefulWidget {
  const BibSearchTab({super.key});

  @override
  State<BibSearchTab> createState() => _BibSearchTabState();
}

class _BibSearchTabState extends State<BibSearchTab> {
  final _controller = TextEditingController();
  bool _searching = false;
  BibSearchResult? _result;

  Future<void> _search() async {
    setState(() => _searching = true);
    final api = Provider.of<AppState>(context, listen: false).api;
    try {
      final data = await api.searchBIB(_controller.text.trim());
      setState(() => _result = BibSearchResult.fromJson(data));
    } on ApiError catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
      }
    } finally {
      if (mounted) setState(() => _searching = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Cari Foto (BIB)')),
      body: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          children: [
            TextField(
              controller: _controller,
              keyboardType: TextInputType.number,
              decoration: InputDecoration(
                labelText: 'Nomor dada (BIB)',
                suffixIcon: IconButton(icon: const Icon(Icons.search), onPressed: _searching ? null : _search),
              ),
              onSubmitted: (_) => _search(),
            ),
            const SizedBox(height: 16),
            if (_searching) const CircularProgressIndicator(),
            if (_result != null && !_searching)
              Expanded(
                child: ListView.builder(
                  itemCount: _result!.photos.length,
                  itemBuilder: (context, i) {
                    final photo = _result!.photos[i];
                    return ListTile(
                      leading: const Icon(Icons.image_outlined),
                      title: Text('Foto ${photo.photoId.substring(0, 8)}'),
                      subtitle: Text('Rp ${photo.priceIdr}'),
                      trailing: photo.bibCheckoutRequiresVerification
                          ? const Tooltip(message: 'Perlu verifikasi OTP peserta', child: Icon(Icons.verified_user_outlined))
                          : null,
                    );
                  },
                ),
              ),
          ],
        ),
      ),
    );
  }
}

class CartTab extends StatelessWidget {
  const CartTab({super.key});

  @override
  Widget build(BuildContext context) {
    final state = context.watch<AppState>();
    final cart = state.cart;
    return Scaffold(
      appBar: AppBar(
        title: const Text('Keranjang'),
        actions: [IconButton(icon: const Icon(Icons.refresh), onPressed: () => state.refreshCart())],
      ),
      body: cart == null || cart.items.isEmpty
          ? const Center(child: Text('Keranjang kosong'))
          : ListView.builder(
              itemCount: cart.items.length,
              itemBuilder: (context, i) {
                final item = cart.items[i];
                return ListTile(
                  leading: const Icon(Icons.image_outlined),
                  title: Text('Foto ${item.photoId.substring(0, 8)}'),
                  subtitle: Text('Rp ${item.priceIdr}'),
                  trailing: IconButton(
                    icon: const Icon(Icons.delete_outline),
                    onPressed: () => state.removeFromCart(item.cartItemId),
                  ),
                );
              },
            ),
      bottomNavigationBar: cart != null && cart.items.isNotEmpty
          ? SafeArea(
              child: Padding(
                padding: const EdgeInsets.all(16),
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.spaceBetween,
                  children: [
                    Text('Total: Rp ${cart.totalAmountIdr}', style: Theme.of(context).textTheme.titleMedium),
                    FilledButton(
                      onPressed: () async {
                        final order = await state.createOrder(cart.items.map((e) => e.photoId).toList());
                        if (order != null && context.mounted) {
                          ScaffoldMessenger.of(context).showSnackBar(
                              SnackBar(content: Text('Pesanan ${order.orderNumber} dibuat: Rp ${order.totalAmountIdr}')));
                        }
                      },
                      child: const Text('Pesan'),
                    ),
                  ],
                ),
              ),
            )
          : null,
    );
  }
}

class CheckoutTab extends StatefulWidget {
  const CheckoutTab({super.key});

  @override
  State<CheckoutTab> createState() => _CheckoutTabState();
}

class _CheckoutTabState extends State<CheckoutTab> {
  PaymentInitiation? _payment;

  @override
  Widget build(BuildContext context) {
    final state = context.watch<AppState>();
    return Scaffold(
      appBar: AppBar(title: const Text('Bayar QRIS')),
      body: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          children: [
            FilledButton(
              onPressed: () async {
                final cart = state.cart;
                if (cart == null || cart.items.isEmpty) {
                  if (mounted) {
                    ScaffoldMessenger.of(context)
                        .showSnackBar(const SnackBar(content: Text('Keranjang kosong')));
                  }
                  return;
                }
                final order = await state.createOrder(cart.items.map((e) => e.photoId).toList());
                if (order == null) return;
                final payment = await state.initiatePayment(order.orderId, 'QRIS');
                if (mounted) setState(() => _payment = payment);
              },
              child: const Text('Bayar dengan QRIS'),
            ),
            const SizedBox(height: 24),
            if (_payment != null) ...[
              Text('Provider: ${_payment!.provider}'),
              Text('Nominal: Rp ${_payment!.amountIdr}', style: Theme.of(context).textTheme.titleLarge),
              const SizedBox(height: 16),
              if (_payment!.qrisString != null)
                SelectableText(_payment!.qrisString!, style: const TextStyle(fontFamily: 'monospace', fontSize: 12))
              else
                const CircularProgressIndicator(),
            ],
          ],
        ),
      ),
    );
  }
}

class CollectionTab extends StatelessWidget {
  const CollectionTab({super.key});

  @override
  Widget build(BuildContext context) {
    final state = context.watch<AppState>();
    if (state.token == null) {
      return const Center(child: Text('Masuk untuk melihat koleksi'));
    }
    return const Center(child: Text('Koleksi foto yang dibeli'));
  }
}
