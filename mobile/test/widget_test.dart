import 'package:flutter_test/flutter_test.dart';
import 'package:galeriku_mobile/src/api.dart';
import 'package:galeriku_mobile/src/state.dart';
import 'package:provider/provider.dart';

import 'package:galeriku_mobile/main.dart';

void main() {
  testWidgets('unauthenticated users see the login screen', (WidgetTester tester) async {
    await tester.pumpWidget(GalerikuApp(api: GalerikuApi(baseUrl: 'http://test')));

    expect(find.text('Masuk'), findsWidgets);
    expect(find.text('Email'), findsOneWidget);
    expect(find.text('Kata Sandi'), findsOneWidget);
  });

  testWidgets('authenticated users see the five-tab shell', (WidgetTester tester) async {
    final api = GalerikuApi(baseUrl: 'http://test');
    final state = AppState(api: api);
    // Simulate a logged-in user by setting the token directly.
    state.setTokenForTest('tok123');

    await tester.pumpWidget(
      ChangeNotifierProvider.value(
        value: state,
        child: GalerikuApp(api: api, authOverride: state),
      ),
    );

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
