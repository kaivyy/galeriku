import 'package:flutter_test/flutter_test.dart';
import 'package:galeriku_mobile/src/api.dart';

import 'package:galeriku_mobile/main.dart';

void main() {
  testWidgets('Home shell renders five tabs', (WidgetTester tester) async {
    await tester.pumpWidget(GalerikuApp(api: GalerikuApi(baseUrl: 'http://test')));

    // The five NavigationDestinations should render.
    expect(find.text('Galeri'), findsOneWidget);
    expect(find.text('Cari'), findsOneWidget);
    expect(find.text('Keranjang'), findsOneWidget);
    expect(find.text('Bayar'), findsOneWidget);
    expect(find.text('Koleksi'), findsOneWidget);

    // Tap the Cari tab.
    await tester.tap(find.text('Cari'));
    await tester.pumpAndSettle();
    expect(find.text('Cari Foto (BIB)'), findsOneWidget);
  });
}
