import {makeScene2D, Circle, Line, Txt, Rect} from '@motion-canvas/2d';
import {all, chain, waitFor, createRef, createRefArray, Vector2, Color} from '@motion-canvas/core';

/**
 * "Identity is Content" animation
 *
 * Shows: A file changes -> its hash changes -> edges from that file invalidate ->
 * surgical re-extraction -> new snapshot root computed.
 *
 * Analogy to git: changing a file produces a new blob hash, which cascades up
 * through trees to the commit hash. Same mechanism, different unit of storage.
 */
export default makeScene2D(function* (view) {
  // Colors
  const bg = '#0f0f0f';
  const nodeColor = '#4ecdc4';
  const edgeColor = '#556677';
  const hashColor = '#95a5a6';
  const changedColor = '#e74c3c';
  const reextractedColor = '#f39c12';
  const freshColor = '#2ecc71';
  const textColor = '#ecf0f1';

  view.fill(bg);

  // Title
  const title = createRef<Txt>();
  view.add(
    <Txt
      ref={title}
      text="Identity is Content"
      fontSize={48}
      fontFamily="JetBrains Mono, monospace"
      fill={textColor}
      y={-320}
      opacity={0}
    />
  );

  // Create file nodes (left column)
  const files = createRefArray<Rect>();
  const fileLabels = ['auth.go', 'store.go', 'context.go'];
  const fileHashes = ['a3f2...', 'b7e1...', 'c9d4...'];
  const filePositions = [
    new Vector2(-400, -120),
    new Vector2(-400, 0),
    new Vector2(-400, 120),
  ];

  for (let i = 0; i < 3; i++) {
    view.add(
      <Rect
        ref={files}
        x={filePositions[i].x}
        y={filePositions[i].y}
        width={160}
        height={60}
        radius={8}
        fill={nodeColor}
        opacity={0}
      >
        <Txt
          text={fileLabels[i]}
          fontSize={16}
          fontFamily="JetBrains Mono, monospace"
          fill={bg}
          y={-8}
        />
        <Txt
          text={fileHashes[i]}
          fontSize={12}
          fontFamily="JetBrains Mono, monospace"
          fill={hashColor}
          y={14}
        />
      </Rect>
    );
  }

  // Create symbol nodes (middle column)
  const symbols = createRefArray<Circle>();
  const symbolLabels = ['ForTask', 'RankSymbols', 'NewStore', 'EdgesTo', 'PackBudget'];
  const symbolPositions = [
    new Vector2(-80, -160),
    new Vector2(-80, -60),
    new Vector2(-80, 40),
    new Vector2(-80, 140),
    new Vector2(-80, 240),
  ];

  for (let i = 0; i < 5; i++) {
    view.add(
      <Circle
        ref={symbols}
        x={symbolPositions[i].x}
        y={symbolPositions[i].y}
        width={80}
        height={80}
        fill={nodeColor}
        opacity={0}
      >
        <Txt
          text={symbolLabels[i]}
          fontSize={11}
          fontFamily="JetBrains Mono, monospace"
          fill={bg}
        />
      </Circle>
    );
  }

  // Create edges (lines between symbols)
  const edges = createRefArray<Line>();
  const edgePairs = [[0, 1], [1, 4], [2, 3], [0, 2]];

  for (const [from, to] of edgePairs) {
    view.add(
      <Line
        ref={edges}
        points={[symbolPositions[from], symbolPositions[to]]}
        stroke={edgeColor}
        lineWidth={2}
        opacity={0}
        endArrow
        arrowSize={8}
      />
    );
  }

  // Snapshot root (right side)
  const snapshotRoot = createRef<Rect>();
  view.add(
    <Rect
      ref={snapshotRoot}
      x={300}
      y={0}
      width={180}
      height={70}
      radius={8}
      fill={nodeColor}
      opacity={0}
    >
      <Txt
        text="Snapshot"
        fontSize={14}
        fontFamily="JetBrains Mono, monospace"
        fill={bg}
        y={-12}
      />
      <Txt
        text="root: f8a2..."
        fontSize={12}
        fontFamily="JetBrains Mono, monospace"
        fill={hashColor}
        y={12}
      />
    </Rect>
  );

  // Subtitle area
  const subtitle = createRef<Txt>();
  view.add(
    <Txt
      ref={subtitle}
      text=""
      fontSize={20}
      fontFamily="JetBrains Mono, monospace"
      fill={hashColor}
      y={340}
    />
  );

  // === Animation sequence ===

  // 1. Fade in title
  yield* title().opacity(1, 0.5);
  yield* waitFor(0.5);

  // 2. Fade in the graph (files, symbols, edges, snapshot)
  yield* subtitle().text('A content-addressed graph at rest.', 0);
  yield* all(
    ...files.map((f, i) => f.opacity(1, 0.3 + i * 0.1)),
    ...symbols.map((s, i) => s.opacity(1, 0.3 + i * 0.08)),
    ...edges.map((e, i) => e.opacity(0.6, 0.4 + i * 0.1)),
    snapshotRoot().opacity(1, 0.6),
  );
  yield* waitFor(1.5);

  // 3. A file changes! auth.go turns red
  yield* subtitle().text('A file changes...', 0.3);
  yield* files[0].fill(changedColor, 0.4);
  yield* waitFor(0.8);

  // 4. Hash invalidation cascades: symbols from that file turn orange
  yield* subtitle().text('Hash changes. Derived nodes invalidate.', 0.3);
  yield* all(
    symbols[0].fill(reextractedColor, 0.3),
    symbols[1].fill(reextractedColor, 0.3),
  );
  yield* all(
    edges[0].stroke(reextractedColor, 0.3),
    edges[1].stroke(reextractedColor, 0.3),
    edges[3].stroke(reextractedColor, 0.3),
  );
  yield* waitFor(1.0);

  // 5. Surgical re-extraction (only affected nodes turn green)
  yield* subtitle().text('Surgical re-extraction. Only affected edges.', 0.3);
  yield* all(
    files[0].fill(freshColor, 0.4),
    symbols[0].fill(freshColor, 0.4),
    symbols[1].fill(freshColor, 0.4),
  );
  yield* all(
    edges[0].stroke(freshColor, 0.3),
    edges[1].stroke(freshColor, 0.3),
    edges[3].stroke(freshColor, 0.3),
  );
  yield* waitFor(0.8);

  // 6. New snapshot root
  yield* subtitle().text('New Merkle root. Graph is current.', 0.3);
  yield* snapshotRoot().fill(freshColor, 0.4);
  yield* waitFor(1.5);

  // 7. Everything fades back to normal
  yield* subtitle().text('No full re-index. Staleness is structural.', 0.3);
  yield* all(
    ...files.map(f => f.fill(nodeColor, 0.5)),
    ...symbols.map(s => s.fill(nodeColor, 0.5)),
    ...edges.map(e => e.stroke(edgeColor, 0.5)),
    snapshotRoot().fill(nodeColor, 0.5),
  );
  yield* waitFor(2.0);
});
