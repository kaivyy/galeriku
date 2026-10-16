import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../src/models.dart';
import '../src/state.dart';

/// QRIS payment screen (UI_UX_SPEC M-15 / W-07).
/// Shows the dynamic QRIS payload from the payment initiation response
/// with the amount and provider, and an idempotency-guarded pay button.
class QrisScreen extends StatefulWidget {
  final PaymentInitiation payment;

  const QrisScreen({super.key, required this.payment});

  @override
  State<QrisScreen> createState() => _QrisScreenState();
}

class _QrisScreenState extends State<QrisScreen> {
  bool _markingPaid = false;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Scaffold(
      appBar: AppBar(title: const Text('Bayar QRIS')),
      body: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Text(
                'Rp ${_formatIdr(widget.payment.amountIdr)}',
                style: theme.textTheme.headlineMedium?.copyWith(fontWeight: FontWeight.w700),
              ),
              const SizedBox(height: 4),
              Text('via ${widget.payment.provider}',
                  style: theme.textTheme.bodySmall?.copyWith(color: theme.colorScheme.onSurfaceVariant)),
              const SizedBox(height: 24),
              Container(
                padding: const EdgeInsets.all(16),
                constraints: const BoxConstraints(maxWidth: 400),
                decoration: BoxDecoration(
                  color: theme.colorScheme.surfaceContainerHighest,
                  borderRadius: BorderRadius.circular(16),
                ),
                child: widget.payment.qrisString != null
                    ? SelectableText(
                        widget.payment.qrisString!,
                        style: const TextStyle(fontFamily: 'monospace', fontSize: 11),
                      )
                    : const CircularProgressIndicator(),
              ),
              const SizedBox(height: 24),
              Text(
                'Scan kode QRIS ini dari aplikasi mobile banking atau e-wallet kamu.',
                textAlign: TextAlign.center,
                style: theme.textTheme.bodySmall?.copyWith(color: theme.colorScheme.onSurfaceVariant),
              ),
              const SizedBox(height: 32),
              FilledButton.icon(
                // Idempotency guard per UI_UX_SPEC section 12: disable after one tap.
                onPressed: _markingPaid ? null : () => _simulatePaid(context),
                style: FilledButton.styleFrom(minimumSize: const Size(0, 48)),
                icon: _markingPaid
                    ? const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2))
                    : const Icon(Icons.check_circle_outline),
                label: const Text('Saya Sudah Bayar'),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Future<void> _simulatePaid(BuildContext context) async {
    setState(() => _markingPaid = true);
    // Real payment status arrives via the Midtrans webhook; until the backend
    // exposes a payment-status endpoint, we return to the app with a notice.
    await Future.delayed(const Duration(milliseconds: 600));
    if (!context.mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
      content: Text('Menunggu konfirmasi pembayaran. Koleksi terbaru muncul otomatis.'),
    ));
    Navigator.of(context).popUntil((route) => route.isFirst);
  }

  String _formatIdr(int amount) {
    final s = amount.toString();
    final buf = StringBuffer();
    for (var i = 0; i < s.length; i++) {
      final remaining = s.length - i;
      buf.write(s[i]);
      if (remaining > 1 && remaining % 3 == 1) buf.write('.');
    }
    return buf.toString();
  }
}

/// Checkout flow entry: creates the order from the cart then opens QRIS.
Future<void> startCheckout(BuildContext context) async {
  final state = context.read<AppState>();
  final cart = state.cart;
  if (cart == null || cart.items.isEmpty) {
    ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Keranjang kosong')));
    return;
  }
  final order = await state.createOrder(cart.items.map((e) => e.photoId).toList());
  if (order == null || !context.mounted) return;
  final payment = await state.initiatePayment(order.orderId, 'QRIS');
  if (payment == null || !context.mounted) return;
  Navigator.of(context).push(MaterialPageRoute(builder: (_) => QrisScreen(payment: payment)));
}
