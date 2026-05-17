import {makeScene2D, Circle, Line, Txt, Rect, Node} from '@motion-canvas/2d';
import {
  all,
  chain,
  waitFor,
  createRef,
  createRefArray,
  Vector2,
  easeInOutCubic,
  linear,
  loop,
  sequence,
} from '@motion-canvas/core';

/**
 * "Identity is Content" — v2
 *
 * Visual narrative:
 * 1. Three source files appear as code blocks with visible hash digests
 * 2. Each file connects to symbol nodes (the graph)
 * 3. Symbol nodes connect upward to a Merkle snapshot root
 * 4. One file's content visibly changes (character edit)
 * 5. Its hash recomputes (digits roll)
 * 6. The invalidation propagates UP the tree (edge pulses, only the affected path)
 * 7. Everything else stays perfectly still (the stillness IS the point)
 * 8. New snapshot root hash settles
 *
 * The spatial layout is a bottom-up tree: files at bottom, symbols in middle,
 * snapshot root at top. This mirrors the Merkle structure.
 */
export default makeScene2D(function* (view) {
  // Palette
  const bg = '#0a0a0f';
  const dim = '#2a2a3a';
  const text = '#e0e0e8';
  const accent = '#4ecdc4';
  const hash = '#6a7a8a';
  const pulse = '#ff6b6b';
  const fresh = '#51cf66';
  const codeBg = '#151520';
  const edgeIdle = '#333344';
  const edgePulse = '#ff6b6b';
  const edgeFresh = '#51cf66';

  view.fill(bg);

  // === LAYER 1: Source files (bottom) ===

  const fileGroup = createRef<Node>();
  view.add(<Node ref={fileGroup} y={200} />);

  // File blocks: show real code with hash underneath
  const fileData = [
    {
      name: 'context.go',
      code: 'func ForTask(desc string) {...}',
      hash: 'sha256: a3f2c8',
      x: -350,
    },
    {
      name: 'store.go',
      code: 'func EdgesTo(h Hash) {...}',
      hash: 'sha256: b7e1d4',
      x: 0,
    },
    {
      name: 'ranking.go',
      code: 'func RankSymbols(s []S) {...}',
      hash: 'sha256: c9d4f1',
      x: 350,
    },
  ];

  const fileRects = createRefArray<Rect>();
  const fileCodeTxts = createRefArray<Txt>();
  const fileHashTxts = createRefArray<Txt>();
  const fileNameTxts = createRefArray<Txt>();

  for (const f of fileData) {
    fileGroup().add(
      <Rect
        ref={fileRects}
        x={f.x}
        width={280}
        height={120}
        radius={6}
        fill={codeBg}
        stroke={dim}
        lineWidth={1}
        opacity={0}
      >
        <Txt
          ref={fileNameTxts}
          text={f.name}
          fontSize={13}
          fontFamily="JetBrains Mono, monospace"
          fill={hash}
          y={-40}
        />
        <Txt
          ref={fileCodeTxts}
          text={f.code}
          fontSize={14}
          fontFamily="JetBrains Mono, monospace"
          fill={text}
          y={-10}
        />
        <Txt
          ref={fileHashTxts}
          text={f.hash}
          fontSize={11}
          fontFamily="JetBrains Mono, monospace"
          fill={hash}
          y={30}
        />
      </Rect>
    );
  }

  // === LAYER 2: Symbol nodes (middle) ===

  const symbolGroup = createRef<Node>();
  view.add(<Node ref={symbolGroup} y={-20} />);

  const symbolData = [
    {name: 'ForTask', x: -280, fileIdx: 0},
    {name: 'RankSymbols', x: -100, fileIdx: 0},
    {name: 'EdgesTo', x: 60, fileIdx: 1},
    {name: 'NewStore', x: 220, fileIdx: 1},
    {name: 'ComputeHITS', x: 380, fileIdx: 2},
  ];

  const symbolCircles = createRefArray<Circle>();

  for (const s of symbolData) {
    symbolGroup().add(
      <Circle
        ref={symbolCircles}
        x={s.x}
        width={70}
        height={70}
        fill={dim}
        opacity={0}
      >
        <Txt
          text={s.name}
          fontSize={10}
          fontFamily="JetBrains Mono, monospace"
          fill={text}
        />
      </Circle>
    );
  }

  // === LAYER 3: Snapshot root (top) ===

  const rootNode = createRef<Rect>();
  const rootHash = createRef<Txt>();

  view.add(
    <Rect
      ref={rootNode}
      y={-220}
      width={220}
      height={70}
      radius={8}
      fill={dim}
      stroke={edgeIdle}
      lineWidth={2}
      opacity={0}
    >
      <Txt
        text="Snapshot Root"
        fontSize={12}
        fontFamily="JetBrains Mono, monospace"
        fill={hash}
        y={-14}
      />
      <Txt
        ref={rootHash}
        text="merkle: f8a2e7"
        fontSize={13}
        fontFamily="JetBrains Mono, monospace"
        fill={accent}
        y={10}
      />
    </Rect>
  );

  // === EDGES: file -> symbols ===

  const fileToSymbolEdges = createRefArray<Line>();

  for (const s of symbolData) {
    const fileX = fileData[s.fileIdx].x;
    fileGroup().add(
      <Line
        ref={fileToSymbolEdges}
        points={[
          new Vector2(fileX, -60),
          new Vector2(s.x, -160),
        ]}
        stroke={edgeIdle}
        lineWidth={1.5}
        opacity={0}
      />
    );
  }

  // === EDGES: symbols -> root ===

  const symbolToRootEdges = createRefArray<Line>();

  for (const s of symbolData) {
    symbolGroup().add(
      <Line
        ref={symbolToRootEdges}
        points={[
          new Vector2(s.x, -40),
          new Vector2(0, -160),
        ]}
        stroke={edgeIdle}
        lineWidth={1.5}
        opacity={0}
      />
    );
  }

  // === SUBTITLE ===

  const subtitle = createRef<Txt>();
  view.add(
    <Txt
      ref={subtitle}
      text=""
      fontSize={18}
      fontFamily="JetBrains Mono, monospace"
      fill={hash}
      y={330}
    />
  );

  // === ANIMATION ===

  // Act 1: Build the graph (bottom up)
  yield* subtitle().text('A content-addressed graph.', 0.3);

  yield* sequence(
    0.1,
    ...fileRects.map(f => f.opacity(1, 0.4)),
  );
  yield* waitFor(0.3);

  yield* sequence(
    0.06,
    ...fileToSymbolEdges.map(e => e.opacity(0.5, 0.3)),
  );
  yield* sequence(
    0.08,
    ...symbolCircles.map(s => s.opacity(1, 0.3)),
  );
  yield* waitFor(0.3);

  yield* sequence(
    0.06,
    ...symbolToRootEdges.map(e => e.opacity(0.5, 0.3)),
  );
  yield* rootNode().opacity(1, 0.4);
  yield* waitFor(1.2);

  // Act 2: A file changes
  yield* subtitle().text('A developer edits context.go...', 0.4);
  yield* waitFor(0.6);

  // The code visibly changes
  yield* fileCodeTxts[0].text('func ForTask(desc string, b int) {...}', 0.6);
  yield* waitFor(0.5);

  // Act 3: Hash recomputes (digits roll)
  yield* subtitle().text('Content changed. Hash recomputes.', 0.4);

  // Flash the hash through intermediate values
  yield* fileHashTxts[0].fill(pulse, 0.2);
  yield* fileHashTxts[0].text('sha256: ......', 0.1);
  yield* waitFor(0.15);
  yield* fileHashTxts[0].text('sha256: d2f1..', 0.1);
  yield* waitFor(0.15);
  yield* fileHashTxts[0].text('sha256: d2f1a9', 0.1);
  yield* fileHashTxts[0].fill(fresh, 0.3);
  yield* fileRects[0].stroke(fresh, 0.3);
  yield* waitFor(0.6);

  // Act 4: Invalidation propagates UP (only affected path)
  yield* subtitle().text('Invalidation propagates. Only the affected path.', 0.4);

  // Pulse edges from file[0] to its symbols (indices 0, 1)
  yield* all(
    fileToSymbolEdges[0].stroke(pulse, 0.3),
    fileToSymbolEdges[1].stroke(pulse, 0.3),
    fileToSymbolEdges[0].opacity(1, 0.3),
    fileToSymbolEdges[1].opacity(1, 0.3),
  );
  yield* all(
    symbolCircles[0].fill(pulse, 0.3),
    symbolCircles[1].fill(pulse, 0.3),
  );
  yield* waitFor(0.3);

  // Pulse from affected symbols to root
  yield* all(
    symbolToRootEdges[0].stroke(pulse, 0.3),
    symbolToRootEdges[1].stroke(pulse, 0.3),
    symbolToRootEdges[0].opacity(1, 0.3),
    symbolToRootEdges[1].opacity(1, 0.3),
  );
  yield* rootNode().stroke(pulse, 0.3);
  yield* waitFor(0.5);

  // Act 5: Re-extraction settles (affected path turns green)
  yield* subtitle().text('Surgical re-extraction. New hashes settle.', 0.4);

  yield* all(
    symbolCircles[0].fill(fresh, 0.4),
    symbolCircles[1].fill(fresh, 0.4),
    fileToSymbolEdges[0].stroke(fresh, 0.4),
    fileToSymbolEdges[1].stroke(fresh, 0.4),
    symbolToRootEdges[0].stroke(fresh, 0.4),
    symbolToRootEdges[1].stroke(fresh, 0.4),
  );

  // Root hash recomputes
  yield* rootHash().text('merkle: ......', 0.1);
  yield* waitFor(0.15);
  yield* rootHash().text('merkle: 2b8c41', 0.15);
  yield* rootNode().stroke(fresh, 0.3);
  yield* waitFor(0.8);

  // Act 6: Emphasis — everything else is STILL
  yield* subtitle().text('Everything else: untouched. No full re-index.', 0.5);
  yield* waitFor(2.0);

  // Act 7: Fade to resting state
  yield* subtitle().text('Identity is content. Staleness is structural.', 0.5);

  yield* all(
    ...symbolCircles.map(s => s.fill(dim, 0.6)),
    ...fileRects.map(f => f.stroke(dim, 0.6)),
    ...fileToSymbolEdges.map(e => e.stroke(edgeIdle, 0.6)),
    ...symbolToRootEdges.map(e => e.stroke(edgeIdle, 0.6)),
    rootNode().stroke(edgeIdle, 0.6),
    fileHashTxts[0].fill(hash, 0.6),
  );
  yield* waitFor(2.0);
});
