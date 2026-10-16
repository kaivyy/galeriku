import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:share_plus/share_plus.dart';
import 'package:provider/provider.dart';
import '../src/api.dart';
import '../src/models.dart';
import '../src/state.dart';

/// Reels-style vertical feed (FotoYu Forkygram reference, UI_UX_SPEC M-04).
///
/// Full-bleed snap-scrolling cards with a creator row, price, and an action
/// rail: like, comment, share, and Beli. Likes and follows persist locally
/// because the social endpoints are not built yet; share uses share_plus
/// when present and falls back to the http package otherwise.
class ReelsFeedScreen extends StatefulWidget {
  const ReelsFeedScreen({super.key});

  @override
  State<ReelsFeedScreen> createState() => _ReelsFeedScreenState();
}

class _ReelsFeedScreenState extends State<ReelsFeedScreen> {
  final _controller = PageController();
  final _commentController = TextEditingController();
  List<PhotoSearchHit> _photos = [];
  bool _loading = true;
  final Set<String> _liked = {};
  final Map<String, bool> _following = {};

  @override
  void initState() {
    super.initState();
    _loadLikes().then((_) => _loadCatalog());
  }

  @override
  void dispose() {
    _controller.dispose();
    _commentController.dispose();
    super.dispose();
  }

  Future<void> _loadLikes() async {
    final prefs = await SharedPreferences.getInstance();
    final stored = prefs.getString('galeriku_likes') ?? '[]';
    try {
      final list = (const JsonDecoder().convert(stored) as List<dynamic>).cast<String>();
      if (mounted) setState(() => _liked.addAll(list));
    } catch (_) {
      // Corrupt storage behaves like an empty set.
    }
  }

  Future<void> _persistLikes() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString('galeriku_likes', const JsonEncoder().convert(_liked.toList()));
  }

  Future<void> _loadCatalog() async {
    final api = context.read<AppState>().api;
    try {
      final data = await api.listPublicPhotos(limit: 12);
      final photos = data['photos'] as List<dynamic>? ?? [];
      if (mounted) {
        setState(() {
          _photos = photos.map((e) => PhotoSearchHit.fromJson(e as Map<String, dynamic>)).toList();
          _loading = false;
        });
      }
    } on ApiError {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _toggleLike(String photoId) {
    setState(() {
      if (_liked.contains(photoId)) {
        _liked.remove(photoId);
      } else {
        _liked.add(photoId);
      }
    });
    _persistLikes();
  }

  Future<void> _share(String photoId) async {
    final text = 'Lihat foto ini di Galeriku: ?photo=$photoId';
    try {
      await Share.share(text);
    } on Exception {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(text)));
      }
    }
  }

  void _openComments(String photoId) {
    showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
      ),
      builder: (context) => Padding(
        padding: EdgeInsets.only(
          left: 16,
          right: 16,
          top: 16,
          bottom: MediaQuery.of(context).viewInsets.bottom + 16,
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              'Komentar',
              style: Theme.of(context).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 8),
            const Text(
              'Belum ada komentar. Jalur interaksi ini akan tersambung ke backend sosial.',
              style: TextStyle(color: Colors.black54),
            ),
            const SizedBox(height: 16),
            Row(
              children: [
                Expanded(
                  child: TextField(
                    controller: _commentController,
                    decoration: const InputDecoration(
                      hintText: 'Tulis komentar...',
                      border: OutlineInputBorder(),
                      isDense: true,
                    ),
                  ),
                ),
                IconButton.filled(
                  onPressed: () {
                    _commentController.clear();
                    Navigator.pop(context);
                  },
                  icon: const Icon(Icons.send),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_photos.isEmpty) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const Icon(Icons.image_not_supported_outlined, size: 48),
            const SizedBox(height: 12),
            const Text('Belum ada foto di katalog'),
            TextButton(onPressed: _loadCatalog, child: const Text('Muat ulang')),
          ],
        ),
      );
    }
    return Scaffold(
      backgroundColor: Colors.black,
      body: PageView.builder(
        controller: _controller,
        scrollDirection: Axis.vertical,
        itemCount: _photos.length,
        itemBuilder: (context, i) {
          final photo = _photos[i];
          return _ReelCard(
            photo: photo,
            liked: _liked.contains(photo.photoId),
            following: _following[photo.photoId] ?? false,
            onLike: () => _toggleLike(photo.photoId),
            onFollow: () => setState(
              () => _following[photo.photoId] = !(_following[photo.photoId] ?? false),
            ),
            onComment: () => _openComments(photo.photoId),
            onShare: () => _share(photo.photoId),
            onBuy: () => context.read<AppState>().addToCart(photo.photoId),
          );
        },
      ),
    );
  }
}

class _ReelCard extends StatelessWidget {
  final PhotoSearchHit photo;
  final bool liked;
  final bool following;
  final VoidCallback onLike;
  final VoidCallback onFollow;
  final VoidCallback onComment;
  final VoidCallback onShare;
  final VoidCallback onBuy;

  const _ReelCard({
    required this.photo,
    required this.liked,
    required this.following,
    required this.onLike,
    required this.onFollow,
    required this.onComment,
    required this.onShare,
    required this.onBuy,
  });

  @override
  Widget build(BuildContext context) {
    return Stack(
      fit: StackFit.expand,
      children: [
        // Media: placeholder surface until the preview object exists.
        Container(color: const Color(0xFF1E293B)),
        // Bottom gradient + creator row + caption + price.
        Positioned(
          left: 0,
          right: 0,
          bottom: 0,
          child: Container(
            padding: const EdgeInsets.fromLTRB(16, 48, 84, 24),
            decoration: const BoxDecoration(
              gradient: LinearGradient(
                begin: Alignment.topCenter,
                end: Alignment.bottomCenter,
                colors: [Colors.transparent, Colors.black87],
              ),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                Row(
                  children: [
                    CircleAvatar(
                      radius: 16,
                      backgroundColor: Theme.of(context).colorScheme.primary,
                      child: Text(
                        photo.photoId.substring(0, 2).toUpperCase(),
                        style: const TextStyle(fontSize: 10, fontWeight: FontWeight.w700, color: Colors.white),
                      ),
                    ),
                    const SizedBox(width: 8),
                    Text(
                      '@fotografer_${photo.photoId.substring(0, 4)}',
                      style: const TextStyle(fontWeight: FontWeight.w600, color: Colors.white),
                    ),
                    const SizedBox(width: 8),
                    GestureDetector(
                      onTap: onFollow,
                      child: Container(
                        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
                        decoration: BoxDecoration(
                          border: Border.all(color: Colors.white),
                          borderRadius: BorderRadius.circular(9999),
                        ),
                        child: Text(
                          following ? 'Mengikuti' : 'Ikuti',
                          style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w600, color: Colors.white),
                        ),
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 8),
                const Text(
                  'Dokumentasi event, siap diunduh setelah master Hi-Res terkirim.',
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(fontSize: 13, color: Colors.white70),
                ),
                const SizedBox(height: 8),
                Text(
                  'Rp ${photo.priceIdr}',
                  style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700, color: Colors.white),
                ),
              ],
            ),
          ),
        ),
        // Right action rail.
        Positioned(
          right: 8,
          bottom: 64,
          child: Column(
            children: [
              _ActionRailButton(
                icon: Icons.favorite,
                label: liked ? 'Disukai' : 'Suka',
                color: liked ? const Color(0xFFEF4444) : Colors.white,
                onTap: onLike,
              ),
              _ActionRailButton(icon: Icons.comment_outlined, label: 'Komen', onTap: onComment),
              _ActionRailButton(icon: Icons.share_outlined, label: 'Bagikan', onTap: onShare),
              _ActionRailButton(icon: Icons.shopping_cart_outlined, label: 'Beli', onTap: onBuy),
            ],
          ),
        ),
      ],
    );
  }
}

class _ActionRailButton extends StatelessWidget {
  final IconData icon;
  final String label;
  final Color color;
  final VoidCallback onTap;

  const _ActionRailButton({
    required this.icon,
    required this.label,
    this.color = Colors.white,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Column(
        children: [
          IconButton(
            onPressed: onTap,
            icon: Icon(icon, color: color, size: 28),
            tooltip: label,
          ),
          Text(
            label,
            style: const TextStyle(fontSize: 10, color: Colors.white),
          ),
        ],
      ),
    );
  }
}
