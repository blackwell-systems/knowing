import {makeScene2D, Circle, Line, Txt, Rect, Node} from '@motion-canvas/2d';
import {
  all,
  chain,
  waitFor,
  createRef,
  createRefArray,
  Vector2,
  sequence,
} from '@motion-canvas/core';

/**
 * "Identity is Content" — 4-Act Storyboard
 *
 * Act 1 — THE PAIN: An agent greps, reads, greps, reads. Token counter climbs.
 *          7 tool calls. "Every session starts from zero."
 *
 * Act 2 — THE ANCHOR: Git's cascade. File -> blob hash -> tree hash -> commit.
 *          "Git solved this for files." Then morph into knowing's structure:
 *          file -> edge hashes -> snapshot root. "Same mechanism. Relationships."
 *
 * Act 3 — THE MECHANISM: A file changes. Hash recomputes. Cascade propagates up
 *          ONLY the affected path. Rest stays still. New root settles.
 *
 * Act 4 — THE PAYOFF: "One call. Ranked context. Always current."
 *          Show context_for_task returning results while token counter stays low.
 */
export default makeScene2D(function* (view) {
  const bg = '#0a0a0f';
  const dim = '#2a2a3a';
  const text = '#e0e0e8';
  const muted = '#6a7a8a';
  const accent = '#4ecdc4';
  const pulse = '#ff6b6b';
  const fresh = '#51cf66';
  const warn = '#ffa94d';
  const codeBg = '#151520';
  const edgeIdle = '#333344';

  view.fill(bg);

  // Reusable subtitle
  const subtitle = createRef<Txt>();
  view.add(
    <Txt
      ref={subtitle}
      text=""
      fontSize={20}
      fontFamily="JetBrains Mono, monospace"
      fill={muted}
      y={340}
    />
  );

  // ============================================================
  // ACT 1: THE PAIN
  // ============================================================

  const actLabel = createRef<Txt>();
  view.add(
    <Txt
      ref={actLabel}
      text=""
      fontSize={14}
      fontFamily="JetBrains Mono, monospace"
      fill={dim}
      y={-340}
    />
  );

  // Token counter (top right)
  const tokenCounter = createRef<Txt>();
  view.add(
    <Txt
      ref={tokenCounter}
      text=""
      fontSize={16}
      fontFamily="JetBrains Mono, monospace"
      fill={warn}
      x={350}
      y={-300}
      opacity={0}
    />
  );

  // Tool call log (center)
  const toolCalls = createRefArray<Txt>();
  const toolCallTexts = [
    '> grep -rn "auth" ./internal/',
    '  ...47 matches (1,200 tokens)',
    '> read internal/auth/session.go',
    '  ...280 lines (2,100 tokens)',
    '> grep -rn "middleware" ./internal/',
    '  ...31 matches (900 tokens)',
    '> read internal/auth/middleware.go',
    '  ...190 lines (1,400 tokens)',
    '> grep -rn "Session" ./internal/',
    '  ...22 matches (650 tokens)',
    '> read internal/mcp/server.go',
    '  ...340 lines (2,500 tokens)',
    '> grep -rn "handler" ./internal/',
    '  ...56 matches (1,680 tokens)',
  ];

  const toolCallGroup = createRef<Node>();
  view.add(<Node ref={toolCallGroup} x={-100} y={-80} opacity={0} />);

  for (let i = 0; i < toolCallTexts.length; i++) {
    toolCallGroup().add(
      <Txt
        ref={toolCalls}
        text={toolCallTexts[i]}
        fontSize={13}
        fontFamily="JetBrains Mono, monospace"
        fill={toolCallTexts[i].startsWith('>') ? text : muted}
        x={0}
        y={i * 22}
        opacity={0}
      />
    );
  }

  // Act 1 animation
  yield* actLabel().text('Act 1: The problem', 0);
  yield* subtitle().text('An agent explores a codebase.', 0.3);
  yield* tokenCounter().opacity(1, 0.3);
  yield* toolCallGroup().opacity(1, 0.3);

  const tokenValues = [0, 1200, 1200, 3300, 3300, 4200, 4200, 5600, 5600, 6250, 6250, 8750, 8750, 10430];

  for (let i = 0; i < toolCallTexts.length; i++) {
    yield* toolCalls[i].opacity(1, 0.15);
    yield* tokenCounter().text(`${tokenValues[i].toLocaleString()} tokens`, 0.1);
    yield* waitFor(0.25);
  }

  yield* waitFor(0.5);
  yield* subtitle().text('7 tool calls. 10,430 tokens. Every session starts from zero.', 0.4);
  yield* waitFor(2.0);

  // Clear Act 1
  yield* all(
    toolCallGroup().opacity(0, 0.4),
    tokenCounter().opacity(0, 0.4),
  );
  yield* waitFor(0.5);

  // ============================================================
  // ACT 2: THE ANCHOR (Git parallel)
  // ============================================================

  yield* actLabel().text('Act 2: Git already solved this', 0.3);
  yield* subtitle().text('', 0);

  // Git's structure (left side)
  const gitGroup = createRef<Node>();
  view.add(<Node ref={gitGroup} x={-250} opacity={0} />);

  const gitBoxes = createRefArray<Rect>();
  const gitLabels = ['file.go', 'blob hash', 'tree hash', 'commit hash'];
  const gitYPositions = [180, 60, -60, -180];

  for (let i = 0; i < 4; i++) {
    gitGroup().add(
      <Rect
        ref={gitBoxes}
        y={gitYPositions[i]}
        width={180}
        height={50}
        radius={6}
        fill={codeBg}
        stroke={accent}
        lineWidth={1}
      >
        <Txt
          text={gitLabels[i]}
          fontSize={14}
          fontFamily="JetBrains Mono, monospace"
          fill={text}
        />
      </Rect>
    );
  }

  // Git edges
  const gitEdges = createRefArray<Line>();
  for (let i = 0; i < 3; i++) {
    gitGroup().add(
      <Line
        ref={gitEdges}
        points={[
          new Vector2(0, gitYPositions[i] - 25),
          new Vector2(0, gitYPositions[i + 1] + 25),
        ]}
        stroke={edgeIdle}
        lineWidth={2}
        endArrow
        arrowSize={8}
      />
    );
  }

  // Git title
  const gitTitle = createRef<Txt>();
  gitGroup().add(
    <Txt
      ref={gitTitle}
      text="Git"
      fontSize={18}
      fontFamily="JetBrains Mono, monospace"
      fill={accent}
      y={-240}
    />
  );

  yield* gitGroup().opacity(1, 0.5);
  yield* subtitle().text('Git: identity is content. Change file, get new hash, cascade up.', 0.4);
  yield* waitFor(2.0);

  // knowing's structure (right side)
  const knowingGroup = createRef<Node>();
  view.add(<Node ref={knowingGroup} x={250} opacity={0} />);

  const knowingBoxes = createRefArray<Rect>();
  const knowingLabels = ['source file', 'edge hashes', 'subgraph', 'snapshot root'];
  const knowingYPositions = [180, 60, -60, -180];

  for (let i = 0; i < 4; i++) {
    knowingGroup().add(
      <Rect
        ref={knowingBoxes}
        y={knowingYPositions[i]}
        width={180}
        height={50}
        radius={6}
        fill={codeBg}
        stroke={fresh}
        lineWidth={1}
      >
        <Txt
          text={knowingLabels[i]}
          fontSize={14}
          fontFamily="JetBrains Mono, monospace"
          fill={text}
        />
      </Rect>
    );
  }

  // knowing edges
  const knowingEdges = createRefArray<Line>();
  for (let i = 0; i < 3; i++) {
    knowingGroup().add(
      <Line
        ref={knowingEdges}
        points={[
          new Vector2(0, knowingYPositions[i] - 25),
          new Vector2(0, knowingYPositions[i + 1] + 25),
        ]}
        stroke={edgeIdle}
        lineWidth={2}
        endArrow
        arrowSize={8}
      />
    );
  }

  // knowing title
  knowingGroup().add(
    <Txt
      text="knowing"
      fontSize={18}
      fontFamily="JetBrains Mono, monospace"
      fill={fresh}
      y={-240}
    />
  );

  // "=" sign between them
  const equalsSign = createRef<Txt>();
  view.add(
    <Txt
      ref={equalsSign}
      text="same model"
      fontSize={16}
      fontFamily="JetBrains Mono, monospace"
      fill={muted}
      x={0}
      y={0}
      opacity={0}
    />
  );

  yield* knowingGroup().opacity(1, 0.5);
  yield* equalsSign().opacity(1, 0.3);
  yield* subtitle().text('Same mechanism. Different unit of storage.', 0.4);
  yield* waitFor(2.5);

  // Clear Act 2
  yield* all(
    gitGroup().opacity(0, 0.4),
    knowingGroup().opacity(0, 0.4),
    equalsSign().opacity(0, 0.4),
  );
  yield* waitFor(0.5);

  // ============================================================
  // ACT 3: THE MECHANISM
  // ============================================================

  yield* actLabel().text('Act 3: The cascade', 0.3);
  yield* subtitle().text('', 0);

  // Build the actual knowing graph (file -> symbols -> root)
  const graphGroup = createRef<Node>();
  view.add(<Node ref={graphGroup} opacity={0} />);

  // Files (bottom)
  const fileData = [
    {name: 'context.go', hash: 'a3f2c8', x: -300, y: 200},
    {name: 'store.go', hash: 'b7e1d4', x: 0, y: 200},
    {name: 'ranking.go', hash: 'c9d4f1', x: 300, y: 200},
  ];

  const gFileRects = createRefArray<Rect>();
  const gFileHashes = createRefArray<Txt>();
  const gFileCode = createRefArray<Txt>();

  for (const f of fileData) {
    graphGroup().add(
      <Rect
        ref={gFileRects}
        x={f.x}
        y={f.y}
        width={240}
        height={90}
        radius={6}
        fill={codeBg}
        stroke={dim}
        lineWidth={1}
      >
        <Txt
          text={f.name}
          fontSize={12}
          fontFamily="JetBrains Mono, monospace"
          fill={muted}
          y={-28}
        />
        <Txt
          ref={gFileCode}
          text={f.name === 'context.go' ? 'func ForTask(desc string)' : ''}
          fontSize={12}
          fontFamily="JetBrains Mono, monospace"
          fill={text}
          y={-5}
        />
        <Txt
          ref={gFileHashes}
          text={f.hash}
          fontSize={11}
          fontFamily="JetBrains Mono, monospace"
          fill={muted}
          y={22}
        />
      </Rect>
    );
  }

  // Symbols (middle)
  const symData = [
    {name: 'ForTask', x: -350, y: 30, fileIdx: 0},
    {name: 'Rank', x: -180, y: 30, fileIdx: 0},
    {name: 'EdgesTo', x: -20, y: 30, fileIdx: 1},
    {name: 'Store', x: 150, y: 30, fileIdx: 1},
    {name: 'HITS', x: 320, y: 30, fileIdx: 2},
  ];

  const gSymbols = createRefArray<Circle>();

  for (const s of symData) {
    graphGroup().add(
      <Circle
        ref={gSymbols}
        x={s.x}
        y={s.y}
        width={60}
        height={60}
        fill={dim}
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

  // Root (top)
  const gRoot = createRef<Rect>();
  const gRootHash = createRef<Txt>();

  graphGroup().add(
    <Rect
      ref={gRoot}
      x={0}
      y={-160}
      width={200}
      height={60}
      radius={8}
      fill={codeBg}
      stroke={dim}
      lineWidth={2}
    >
      <Txt
        text="snapshot"
        fontSize={11}
        fontFamily="JetBrains Mono, monospace"
        fill={muted}
        y={-12}
      />
      <Txt
        ref={gRootHash}
        text="f8a2e7"
        fontSize={14}
        fontFamily="JetBrains Mono, monospace"
        fill={accent}
        y={10}
      />
    </Rect>
  );

  // Edges: file -> symbol
  const gFileEdges = createRefArray<Line>();
  for (const s of symData) {
    graphGroup().add(
      <Line
        ref={gFileEdges}
        points={[
          new Vector2(fileData[s.fileIdx].x, fileData[s.fileIdx].y - 45),
          new Vector2(s.x, s.y + 30),
        ]}
        stroke={edgeIdle}
        lineWidth={1.5}
      />
    );
  }

  // Edges: symbol -> root
  const gSymEdges = createRefArray<Line>();
  for (const s of symData) {
    graphGroup().add(
      <Line
        ref={gSymEdges}
        points={[
          new Vector2(s.x, s.y - 30),
          new Vector2(0, -130),
        ]}
        stroke={edgeIdle}
        lineWidth={1.5}
      />
    );
  }

  // Show the graph
  yield* graphGroup().opacity(1, 0.6);
  yield* subtitle().text('The graph at rest. Every hash is current.', 0.4);
  yield* waitFor(1.5);

  // THE EDIT
  yield* subtitle().text('A developer adds a parameter...', 0.4);
  yield* waitFor(0.5);
  yield* gFileCode[0].text('func ForTask(desc string, n int)', 0.5);
  yield* waitFor(0.6);

  // HASH RECOMPUTES
  yield* subtitle().text('Content changed. New hash.', 0.3);
  yield* gFileHashes[0].fill(pulse, 0.2);
  yield* gFileHashes[0].text('......', 0.1);
  yield* waitFor(0.2);
  yield* gFileHashes[0].text('d2f1a9', 0.15);
  yield* gFileHashes[0].fill(fresh, 0.3);
  yield* gFileRects[0].stroke(fresh, 0.3);
  yield* waitFor(0.5);

  // CASCADE UP (only affected path: file[0] -> sym[0], sym[1] -> root)
  yield* subtitle().text('Cascade up. Only the affected path.', 0.3);

  // Dim unaffected nodes
  yield* all(
    gFileRects[1].opacity(0.3, 0.4),
    gFileRects[2].opacity(0.3, 0.4),
    gSymbols[2].opacity(0.3, 0.4),
    gSymbols[3].opacity(0.3, 0.4),
    gSymbols[4].opacity(0.3, 0.4),
    gFileEdges[2].opacity(0.2, 0.4),
    gFileEdges[3].opacity(0.2, 0.4),
    gFileEdges[4].opacity(0.2, 0.4),
    gSymEdges[2].opacity(0.2, 0.4),
    gSymEdges[3].opacity(0.2, 0.4),
    gSymEdges[4].opacity(0.2, 0.4),
  );

  // Pulse affected edges
  yield* all(
    gFileEdges[0].stroke(pulse, 0.3),
    gFileEdges[1].stroke(pulse, 0.3),
  );
  yield* all(
    gSymbols[0].fill(pulse, 0.3),
    gSymbols[1].fill(pulse, 0.3),
  );
  yield* waitFor(0.3);
  yield* all(
    gSymEdges[0].stroke(pulse, 0.3),
    gSymEdges[1].stroke(pulse, 0.3),
  );
  yield* gRoot().stroke(pulse, 0.3);
  yield* waitFor(0.4);

  // Settle to green
  yield* all(
    gFileEdges[0].stroke(fresh, 0.4),
    gFileEdges[1].stroke(fresh, 0.4),
    gSymbols[0].fill(fresh, 0.4),
    gSymbols[1].fill(fresh, 0.4),
    gSymEdges[0].stroke(fresh, 0.4),
    gSymEdges[1].stroke(fresh, 0.4),
    gRoot().stroke(fresh, 0.4),
  );

  // New root hash
  yield* gRootHash().text('......', 0.1);
  yield* waitFor(0.15);
  yield* gRootHash().text('2b8c41', 0.15);
  yield* waitFor(1.0);

  yield* subtitle().text('2 edges re-extracted. 5 untouched. No full re-index.', 0.4);
  yield* waitFor(2.5);

  // Clear Act 3
  yield* graphGroup().opacity(0, 0.5);
  yield* waitFor(0.5);

  // ============================================================
  // ACT 4: THE PAYOFF
  // ============================================================

  yield* actLabel().text('Act 4: The result', 0.3);
  yield* subtitle().text('', 0);

  // Show the single tool call
  const payoffGroup = createRef<Node>();
  view.add(<Node ref={payoffGroup} opacity={0} />);

  const payoffCall = createRef<Txt>();
  payoffGroup().add(
    <Txt
      ref={payoffCall}
      text='> context_for_task("refactor auth middleware")'
      fontSize={16}
      fontFamily="JetBrains Mono, monospace"
      fill={accent}
      y={-80}
    />
  );

  // Result block
  const payoffResult = createRefArray<Txt>();
  const resultLines = [
    '  ForTask        score: 0.94  (authority)',
    '  RankSymbols    score: 0.87  (hub)',
    '  AuthMiddleware score: 0.82  (blast radius: 12)',
    '  SessionStore   score: 0.71  (runtime-confirmed)',
    '  HandleLogin    score: 0.65  (feedback: +3)',
  ];

  for (let i = 0; i < resultLines.length; i++) {
    payoffGroup().add(
      <Txt
        ref={payoffResult}
        text={resultLines[i]}
        fontSize={13}
        fontFamily="JetBrains Mono, monospace"
        fill={text}
        y={-30 + i * 28}
        opacity={0}
      />
    );
  }

  // Token comparison
  const payoffTokens = createRef<Txt>();
  payoffGroup().add(
    <Txt
      ref={payoffTokens}
      text=""
      fontSize={16}
      fontFamily="JetBrains Mono, monospace"
      fill={fresh}
      y={200}
    />
  );

  yield* payoffGroup().opacity(1, 0.4);
  yield* subtitle().text('One call. Ranked by graph structure + runtime + feedback.', 0.4);
  yield* waitFor(0.5);

  // Results appear one by one
  for (let i = 0; i < resultLines.length; i++) {
    yield* payoffResult[i].opacity(1, 0.2);
    yield* waitFor(0.2);
  }

  yield* waitFor(0.5);
  yield* payoffTokens().text('1 tool call. 2,400 tokens. Always current.', 0.3);
  yield* waitFor(1.0);
  yield* subtitle().text('vs 7 calls, 10,430 tokens, possibly stale.', 0.4);
  yield* waitFor(3.0);

  // Final tagline
  yield* all(
    payoffGroup().opacity(0, 0.5),
    subtitle().text('', 0.3),
    actLabel().text('', 0.3),
  );
  yield* waitFor(0.3);

  const tagline = createRef<Txt>();
  view.add(
    <Txt
      ref={tagline}
      text="Identity is content. The graph updates itself."
      fontSize={28}
      fontFamily="JetBrains Mono, monospace"
      fill={text}
      opacity={0}
    />
  );
  yield* tagline().opacity(1, 0.6);
  yield* waitFor(3.0);
});
