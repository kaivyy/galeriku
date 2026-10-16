import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../src/api.dart';
import '../src/models.dart';
import '../src/state.dart';

/// GaleriGram feed screen (UI_UX_SPEC M-04) with live catalog (M-08 BIB search).
class FeedScreen extends StatefulWidget {
  const FeedScreen({super.key});

  @override
  State<FeedScreen> createState() => _FeedScreenState();
}

class _FeedScreenState extends State<FeedScreen> {
  final _searchController = TextEditingController();
  bool _searching = false;
  bool _loadingCatalog = true;
  List<PhotoSearchHit> _catalog = [];
  BibSearchResult? _result;

  @override
  void initState() {
    super.initState();
    _loadCatalog();
  }

  @override
  void dispose() {
    _searchController.dispose();
    super.dispose();
  }

  Future<void> _loadCatalog() async {
    final api = context.read<AppState>().api;
    try {
      final data = await api.listPublicPhotos(limit: 12);
      final photos = data['photos'] as List<dynamic>? ?? [];
      if (mounted) {
        setState(() {
          _catalog = photos
              .map((e) => PhotoSearchHit.fromJson(e as Map<String, dynamic>))
              .toList();
          _loadingCatalog = false;
        });
      }
    } on ApiError {
      if (mounted) setState(() => _loadingCatalog = false);
    }
  }

  Future<void> _search() async {
    final bib = _searchController.text.trim();
    if (bib.isEmpty) return;
    setState(() => _searching = true);
    final api = context.read<AppState>().api;
    try {
      final data = await api.searchBIB(bib);
      if (mounted) setState(() => _result = BibSearchResult.fromJson(data));
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
      appBar: AppBar(
        title: const Text('Galeriku'),
        actions: [
          IconButton(
            icon: const Icon(Icons.search),
            tooltip: 'Cari BIB',
            onPressed: () => _showSearchSheet(context),
          ),
        ],
      ),
      body: _searching
          ? const Center(child: CircularProgressIndicator())
          : _result != null
              ? _buildResults()
              : _buildCatalogGrid(),
    );
  }

  Widget _buildCatalogGrid() {
    if (_loadingCatalog) {
      return GridView.builder(
        padding: const EdgeInsets.all(8),
        gridDelegate: const SliverGridDelegateWithFixedCrossAxisCount(
          crossAxisCount: 2, mainAxisSpacing: 8, crossAxisSpacing: 8, childAspectRatio: 0.75,
        ),
        itemCount: 8,
        itemBuilder: (context, i) => Container(
          decoration: BoxDecoration(
            color: Theme.of(context).colorScheme.surfaceContainerHighest,
            borderRadius: BorderRadius.circular(8),
          ),
        ),
      );
    }
    if (_catalog.isEmpty) return _buildFeedPlaceholder();
    return RefreshIndicator(
      onRefresh: _loadCatalog,
      child: GridView.builder(
        padding: const EdgeInsets.all(8),
        gridDelegate: const SliverGridDelegateWithFixedCrossAxisCount(
          crossAxisCount: 2, mainAxisSpacing: 8, crossAxisSpacing: 8, childAspectRatio: 0.75,
        ),
        itemCount: _catalog.length,
        itemBuilder: (context, i) {
          final photo = _catalog[i];
          return _PhotoCard(
            photo: photo,
            onAdd: () => context.read<AppState>().addToCart(photo.photoId),
          );
        },
      ),
    );
  }

  Widget _buildFeedPlaceholder() {
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

  Widget _buildResults() {
    final photos = _result!.photos;
    if (photos.isEmpty) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const Icon(Icons.search_off, size: 48),
            const SizedBox(height: 12),
            const Text('Belum ada foto yang cocok'),
            TextButton(
              onPressed: () => setState(() => _result = null),
              child: const Text('Cari lagi'),
            ),
          ],
        ),
      );
    }
    return GridView.builder(
      padding: const EdgeInsets.all(8),
      gridDelegate: const SliverGridDelegateWithFixedCrossAxisCount(
        crossAxisCount: 2,
        mainAxisSpacing: 8,
        crossAxisSpacing: 8,
        childAspectRatio: 0.75,
      ),
      itemCount: photos.length,
      itemBuilder: (context, i) {
        final photo = photos[i];
        return _PhotoCard(
          photo: photo,
          onAdd: () => context.read<AppState>().addToCart(photo.photoId),
        );
      },
    );
  }

  void _showSearchSheet(BuildContext context) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (context) => Padding(
        padding: EdgeInsets.only(
          left: 16,
          right: 16,
          top: 16,
          bottom: MediaQuery.of(context).viewInsets.bottom + 16,
        ),
        child: Row(
          children: [
            Expanded(
              child: TextField(
                controller: _searchController,
                keyboardType: TextInputType.number,
                autofocus: true,
                decoration: const InputDecoration(
                  labelText: 'Nomor dada (BIB)',
                  border: OutlineInputBorder(),
                ),
                onSubmitted: (_) {
                  Navigator.pop(context);
                  _search();
                },
              ),
            ),
            const SizedBox(width: 8),
            IconButton.filled(
              icon: const Icon(Icons.search),
              onPressed: () {
                Navigator.pop(context);
                _search();
              },
            ),
          ],
        ),
      ),
    );
  }
}

class _PhotoCard extends StatelessWidget {
  final PhotoSearchHit photo;
  final VoidCallback onAdd;

  const _PhotoCard({required this.photo, required this.onAdd});

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Card(
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onAdd,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Expanded(
              child: Container(
                color: theme.colorScheme.surfaceContainerHighest,
                child: const Center(child: Icon(Icons.image_outlined, size: 32)),
              ),
            ),
            Padding(
              padding: const EdgeInsets.all(8),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  Text(
                    'Rp ${photo.priceIdr}',
                    style: theme.textTheme.titleSmall?.copyWith(fontWeight: FontWeight.w700),
                  ),
                  if (photo.bibCheckoutRequiresVerification)
                    Icon(Icons.verified_user_outlined,
                        size: 16, color: theme.colorScheme.tertiary),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}
