import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../src/api.dart';
import '../src/state.dart';

/// Profile tab with the dual balance card (UI_UX_SPEC M-19/M-20;
/// FotoYu-referenced Available vs Pending Settlement split).
class ProfileScreen extends StatefulWidget {
  const ProfileScreen({super.key});

  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> {
  bool _loadingWallet = true;
  int _availableIdr = 0;
  int _pendingHoldIdr = 0;
  String? _walletError;

  @override
  void initState() {
    super.initState();
    _loadWallet();
  }

  Future<void> _loadWallet() async {
    final api = context.read<AppState>().api;
    try {
      final data = await api.getWallet();
      if (mounted) {
        setState(() {
          _availableIdr = (data['available_balance_idr'] as num?)?.toInt() ?? 0;
          _pendingHoldIdr = (data['pending_hold_balance_idr'] as num?)?.toInt() ?? 0;
          _loadingWallet = false;
        });
      }
    } on ApiError catch (e) {
      if (mounted) {
        setState(() {
          _walletError = e.message;
          _loadingWallet = false;
        });
      }
    }
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

  @override
  Widget build(BuildContext context) {
    final state = context.watch<AppState>();
    final theme = Theme.of(context);
    return Scaffold(
      appBar: AppBar(
        title: const Text('Profil'),
        actions: [
          IconButton(
            icon: const Icon(Icons.logout),
            tooltip: 'Keluar',
            onPressed: () => state.logout(),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: _loadWallet,
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            // Account header
            Row(
              children: [
                CircleAvatar(
                  radius: 28,
                  child: Text(
                    'G',
                    style: theme.textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w700),
                  ),
                ),
                const SizedBox(width: 16),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text('Akun Galeriku', style: theme.textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w700)),
                      Text('Pembeli | Kreator | Host', style: theme.textTheme.bodySmall?.copyWith(color: theme.colorScheme.onSurfaceVariant)),
                    ],
                  ),
                ),
              ],
            ),
            const SizedBox(height: 24),
            // Dual balance card
            Card(
              child: Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text('Dompet Saya', style: theme.textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w700)),
                    const SizedBox(height: 12),
                    if (_loadingWallet)
                      const Center(child: CircularProgressIndicator())
                    else if (_walletError != null)
                      Text(_walletError!, style: TextStyle(color: theme.colorScheme.error))
                    else ...[
                      Row(
                        children: [
                          Expanded(
                            child: _BalanceTile(
                              label: 'Tersedia',
                              amount: 'Rp ${_formatIdr(_availableIdr)}',
                              color: theme.colorScheme.primary,
                            ),
                          ),
                          const SizedBox(width: 12),
                          Expanded(
                            child: _BalanceTile(
                              label: 'Tertahan (T+7)',
                              amount: 'Rp ${_formatIdr(_pendingHoldIdr)}',
                              color: theme.colorScheme.tertiary,
                            ),
                          ),
                        ],
                      ),
                    ],
                  ],
                ),
              ),
            ),
            const SizedBox(height: 16),
            // Actions
            Card(
              child: Column(
                children: [
                  ListTile(
                    leading: const Icon(Icons.collections_bookmark_outlined),
                    title: const Text('Koleksi Saya'),
                    trailing: const Icon(Icons.chevron_right),
                    onTap: () {},
                  ),
                  ListTile(
                    leading: const Icon(Icons.account_balance_outlined),
                    title: const Text('Rekening Bank'),
                    trailing: const Icon(Icons.chevron_right),
                    onTap: () {},
                  ),
                  ListTile(
                    leading: const Icon(Icons.security_outlined),
                    title: const Text('Keamanan & Privasi'),
                    trailing: const Icon(Icons.chevron_right),
                    onTap: () {},
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _BalanceTile extends StatelessWidget {
  final String label;
  final String amount;
  final Color color;

  const _BalanceTile({required this.label, required this.amount, required this.color});

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: theme.colorScheme.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label, style: theme.textTheme.bodySmall?.copyWith(color: theme.colorScheme.onSurfaceVariant)),
          const SizedBox(height: 4),
          Text(amount, style: theme.textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w700, color: color)),
        ],
      ),
    );
  }
}
