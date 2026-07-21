import 'package:fake_async/fake_async.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lanos/services/batched_notifier.dart';

void main() {
  group('formatFileBatch', () {
    final l10n = (
      singleTitle: () => 'File received',
      multiTitle: (int n) => '$n files received',
      andMore: (String first5, int n) => '$first5 and $n more',
    );

    test('empty list returns empty strings', () {
      final r = formatFileBatch([], l10n);
      expect(r.title, '');
      expect(r.body, '');
    });

    test('single file', () {
      final r = formatFileBatch(['report.pdf'], l10n);
      expect(r.title, 'File received');
      expect(r.body, 'report.pdf');
    });

    test('two files joined with 、', () {
      final r = formatFileBatch(['a.txt', 'b.txt'], l10n);
      expect(r.title, '2 files received');
      expect(r.body, 'a.txt、b.txt');
    });

    test('exactly 5 files lists all', () {
      final files = ['f1', 'f2', 'f3', 'f4', 'f5'];
      final r = formatFileBatch(files, l10n);
      expect(r.title, '5 files received');
      expect(r.body, 'f1、f2、f3、f4、f5');
    });

    test('more than 5 truncates with andMore', () {
      final files = ['f1', 'f2', 'f3', 'f4', 'f5', 'f6'];
      final r = formatFileBatch(files, l10n);
      expect(r.title, '6 files received');
      expect(r.body, 'f1、f2、f3、f4、f5 and 6 more');
    });
  });

  group('BatchedNotifier', () {
    test('single file flushes after batchWindow', () {
      fakeAsync((async) {
        final flushes = <List<String>>[];
        final bn = BatchedNotifier(
          onFlush: flushes.add,
          batchWindow: const Duration(milliseconds: 800),
        );

        bn.add('single.txt');
        expect(flushes, isEmpty);

        async.elapse(const Duration(milliseconds: 799));
        expect(flushes, isEmpty);

        async.elapse(const Duration(milliseconds: 1));
        expect(flushes.length, 1);
        expect(flushes.single, ['single.txt']);
      });
    });

    test('batch fills at maxBatchSize and flushes immediately', () {
      fakeAsync((async) {
        final flushes = <List<String>>[];
        final bn = BatchedNotifier(
          onFlush: flushes.add,
          batchWindow: const Duration(seconds: 10),
          maxBatchSize: 5,
        );

        for (var i = 1; i <= 4; i++) {
          bn.add('f$i.txt');
        }
        expect(flushes, isEmpty);

        bn.add('f5.txt');
        expect(flushes.length, 1);
        expect(
          flushes.single,
          ['f1.txt', 'f2.txt', 'f3.txt', 'f4.txt', 'f5.txt'],
        );
      });
    });

    test('partial batch flushes on timer', () {
      fakeAsync((async) {
        final flushes = <List<String>>[];
        final bn = BatchedNotifier(
          onFlush: flushes.add,
          batchWindow: const Duration(milliseconds: 500),
        );

        bn.add('a.txt');
        bn.add('b.txt');
        bn.add('c.txt');
        expect(flushes, isEmpty);

        async.elapse(const Duration(milliseconds: 500));
        expect(flushes.length, 1);
        expect(flushes.single, ['a.txt', 'b.txt', 'c.txt']);
      });
    });

    test('timer restarts on each add (debounce)', () {
      fakeAsync((async) {
        final flushes = <List<String>>[];
        final bn = BatchedNotifier(
          onFlush: flushes.add,
          batchWindow: const Duration(milliseconds: 500),
        );

        bn.add('a.txt');
        async.elapse(const Duration(milliseconds: 400));

        bn.add('b.txt');
        async.elapse(const Duration(milliseconds: 400));
        expect(flushes, isEmpty);

        async.elapse(const Duration(milliseconds: 100));
        expect(flushes.length, 1);
        expect(flushes.single, ['a.txt', 'b.txt']);
      });
    });

    test('multiple batches: full flush then timer flush', () {
      fakeAsync((async) {
        final flushes = <List<String>>[];
        final bn = BatchedNotifier(
          onFlush: flushes.add,
          batchWindow: const Duration(milliseconds: 500),
          maxBatchSize: 5,
        );

        for (var i = 1; i <= 5; i++) {
          bn.add('b1_$i.txt');
        }
        expect(flushes.length, 1);

        bn.add('b2_1.txt');
        bn.add('b2_2.txt');
        expect(flushes.length, 1);

        async.elapse(const Duration(milliseconds: 500));
        expect(flushes.length, 2);
        expect(flushes[1], ['b2_1.txt', 'b2_2.txt']);
      });
    });

    test('flush() forces immediate flush', () {
      final flushes = <List<String>>[];
      final bn = BatchedNotifier(
        onFlush: flushes.add,
        batchWindow: const Duration(seconds: 10),
      );

      bn.add('a.txt');
      bn.add('b.txt');
      expect(flushes, isEmpty);

      bn.flush();
      expect(flushes.length, 1);
      expect(flushes.single, ['a.txt', 'b.txt']);
    });

    test('dispose flushes remaining and blocks future adds', () {
      final flushes = <List<String>>[];
      final bn = BatchedNotifier(
        onFlush: flushes.add,
        batchWindow: const Duration(seconds: 10),
      );

      bn.add('a.txt');
      bn.add('b.txt');
      expect(flushes, isEmpty);

      bn.dispose();
      expect(flushes.length, 1);
      expect(flushes.single, ['a.txt', 'b.txt']);

      bn.add('c.txt');
      expect(flushes.length, 1);
    });

    test('dispose is idempotent', () {
      final flushes = <List<String>>[];
      final bn = BatchedNotifier(onFlush: flushes.add);

      bn.add('a.txt');
      bn.dispose();
      bn.dispose();
      expect(flushes.length, 1);
    });

    test('empty flush is a no-op', () {
      final flushes = <List<String>>[];
      final bn = BatchedNotifier(onFlush: flushes.add);

      bn.flush();
      expect(flushes, isEmpty);
    });
  });
}
