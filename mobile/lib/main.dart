/// Galeriku mobile app entry point.
///
/// Five-tab layout per the Phase 8.1 acceptance criteria: feed (GaleriGram),
/// AI photo search (selfie/BIB), cart, QRIS payment, and collections.
library;

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import 'src/api.dart';
import 'src/state.dart';
import 'screens/login_screen.dart';
import 'screens/feed_screen.dart';
import 'screens/qris_screen.dart';
import 'screens/profile_screen.dart';

void main() {
  final api = GalerikuApi(baseUrl: const String.fromEnvironment('API_BASE_URL', defaultValue: 'http://localhost:8080'));
  runApp(GalerikuApp(api: api));
}

class GalerikuApp extends StatelessWidget {
  final GalerikuApi api;
  final AppState? authOverride;

  const GalerikuApp({super.key, required this.api, this.authOverride});

  @override
  Widget build(BuildContext context) {
    return ChangeNotifierProvider(
      create: (_) => authOverride ?? AppState(api: api),
      child: MaterialApp(
        title: 'Galeriku',
        theme: ThemeData(colorSchemeSeed: const Color(0xFF4F46E5), useMaterial3: true),
        darkTheme: ThemeData(
          colorSchemeSeed: const Color(0xFF4F46E5),
          brightness: Brightness.dark,
          scaffoldBackgroundColor: const Color(0xFF0F172A),
          useMaterial3: true,
        ),
        themeMode: ThemeMode.dark,
        home: const AppGate(),
      ),
    );
  }
}

/// Shows login when no token, home shell when logged in.
class AppGate extends StatelessWidget {
  const AppGate({super.key});

  @override
  Widget build(BuildContext context) {
    final state = context.watch<AppState>();
    if (state.token == null) return const LoginScreen();
    return const HomeShell();
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
    FeedScreen(),
    FeedScreen(),
    CartTab(),
    CheckoutTab(),
    ProfileScreen(),
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

class CheckoutTab extends StatelessWidget {
  const CheckoutTab({super.key});

  @override
  Widget build(BuildContext context) {
    final state = context.watch<AppState>();
    final cart = state.cart;
    return Scaffold(
      appBar: AppBar(title: const Text('Bayar')),
      body: Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              if (cart != null && cart.items.isNotEmpty) ...[
                Text('Total: Rp ${cart.totalAmountIdr}',
                    style: Theme.of(context).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w700)),
                const SizedBox(height: 16),
                FilledButton.icon(
                  onPressed: () => startCheckout(context),
                  icon: const Icon(Icons.qr_code),
                  label: const Text('Bayar dengan QRIS'),
                  style: FilledButton.styleFrom(minimumSize: const Size(0, 48)),
                ),
              ] else
                const Text('Keranjang kosong. Cari foto dulu.'),
            ],
          ),
        ),
      ),
    );
  }
}
