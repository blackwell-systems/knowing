import {makeScene2D, Circle, Line, Txt, Rect, Node} from '@motion-canvas/2d';
import {
  all,
  chain,
  waitFor,
  createRef,
  createRefArray,
  Vector2,
  sequence,
  easeInOutCubic,
  easeOutCubic,
  easeInCubic,
} from '@motion-canvas/core';

/**
 * "Identity is Content" — 4-Act, Polished
 */
export default makeScene2D(function* (view) {
  const bg = '#08080c';
  const dim = '#1e1e2a';
  const text = '#e8e8f0';
  const muted = '#5a6a7a';
  const accent = '#4ecdc4';
  const pulse = '#ff6b6b';
  const fresh = '#51cf66';
  const warn = '#ffa94d';
  const codeBg = '#12121c';
  const edgeIdle = '#282838';
  const gridColor = '#0f0f18';

  view.fill(bg);

  // Background dot grid for depth
  const gridGroup = createRef<Node>();
  view.add(<Node ref={gridGroup} opacity={0.4} />);
  for (let x = -800; x <= 800; x += 60) {
    for (let y = -400; y <= 400; y += 60) {
      gridGroup().add(
        <Circle x={x} y={y} width={2} height={2} fill={gridColor} />
      );
    }
  }

  // Reusable subtitle with shadow effect
  const subtitle = createRef<Txt>();
  view.add(
    <Txt
      ref={subtitle}
      text=""
      fontSize={20}
      fontFamily="'JetBrains Mono', 'Fira Code', monospace"
      fontWeight={300}
      fill={muted}
      y={340}
    />
  );

  const actLabel = createRef<Txt>();
  view.add(
    <Txt
      ref={actLabel}
      text=""
      fontSize={14}
      fontFamily="'JetBrains Mono', monospace"
      fontWeight={700}
      fill={accent}
      y={-350}
      letterSpacing={3}
    />
  );

  // ============================================================
  // ACT 1: THE PAIN
  // ============================================================

  // Token counter (top right) with glow
  const tokenCounter = createRef<Txt>();
  view.add(
    <Txt
      ref={tokenCounter}
      text=""
      fontSize={18}
      fontFamily="'JetBrains Mono', monospace"
      fontWeight={700}
      fill={warn}
      x={380}
      y={-310}
      opacity={0}
    />
  );

  // Tool call log
  const toolCallGroup = createRef<Node>();
  view.add(<Node ref={toolCallGroup} x={-80} y={-100} opacity={0} />);

  const toolCallTexts = [
    {t: '> grep -rn "auth" ./internal/', cmd: true},
    {t: '  ...47 matches', cmd: false},
    {t: '> read internal/auth/session.go', cmd: true},
    {t: '  ...280 lines', cmd: false},
    {t: '> grep -rn "middleware" ./', cmd: true},
    {t: '  ...31 matches', cmd: false},
    {t: '> read internal/auth/middleware.go', cmd: true},
    {t: '  ...190 lines', cmd: false},
    {t: '> grep -rn "Session" ./internal/', cmd: true},
    {t: '  ...22 matches', cmd: false},
    {t: '> read internal/mcp/server.go', cmd: true},
    {t: '  ...340 lines', cmd: false},
    {t: '> grep -rn "handler" ./internal/', cmd: true},
    {t: '  ...56 matches', cmd: false},
  ];

  const toolCalls = createRefArray<Txt>();
  for (let i = 0; i < toolCallTexts.length; i++) {
    toolCallGroup().add(
      <Txt
        ref={toolCalls}
        text={toolCallTexts[i].t}
        fontSize={14}
        fontFamily="'JetBrains Mono', monospace"
        fill={toolCallTexts[i].cmd ? text : muted}
        y={i * 24}
        opacity={0}
      />
    );
  }

  // --- Animate Act 1 ---
  yield* actLabel().text('THE PROBLEM', 0);
  yield* subtitle().text('An agent explores a codebase.', 0.5);
  yield* waitFor(0.3);
  yield* all(
    tokenCounter().opacity(1, 0.4),
    toolCallGroup().opacity(1, 0.4),
  );

  const tokenValues = [0, 1200, 1200, 3300, 3300, 4200, 4200, 5600, 5600, 6250, 6250, 8750, 8750, 10430];

  for (let i = 0; i < toolCallTexts.length; i++) {
    yield* toolCalls[i].opacity(1, 0.12);
    yield* tokenCounter().text(`${tokenValues[i].toLocaleString()} tokens`, 0.08);
    if (toolCallTexts[i].cmd) {
      yield* waitFor(0.3);
    } else {
      yield* waitFor(0.15);
    }
  }

  yield* waitFor(0.3);
  // Flash token counter red
  yield* tokenCounter().fill(pulse, 0.3);
  yield* subtitle().text('7 tool calls. 10,430 tokens. Possibly stale.', 0.5);
  yield* waitFor(2.5);

  // Fade out Act 1
  yield* all(
    toolCallGroup().opacity(0, 0.6),
    tokenCounter().opacity(0, 0.6),
    subtitle().text('', 0.3),
  );
  yield* waitFor(0.4);

  // ============================================================
  // ACT 2: THE ANCHOR
  // ============================================================

  yield* actLabel().text('THE INSIGHT', 0.3);

  // Git cascade (left)
  const gitGroup = createRef<Node>();
  view.add(<Node ref={gitGroup} x={-260} opacity={0} />);

  const gitLevels = [
    {label: 'file content', y: 160, color: accent},
    {label: 'blob hash', y: 53, color: accent},
    {label: 'tree hash', y: -53, color: accent},
    {label: 'commit hash', y: -160, color: accent},
  ];

  const gitBoxes = createRefArray<Rect>();
  for (const lvl of gitLevels) {
    gitGroup().add(
      <Rect
        ref={gitBoxes}
        y={lvl.y}
        width={170}
        height={46}
        radius={6}
        fill={codeBg}
        stroke={lvl.color}
        lineWidth={1.5}
      >
        <Txt
          text={lvl.label}
          fontSize={13}
          fontFamily="'JetBrains Mono', monospace"
          fill={text}
        />
      </Rect>
    );
  }

  const gitEdges = createRefArray<Line>();
  for (let i = 0; i < 3; i++) {
    gitGroup().add(
      <Line
        ref={gitEdges}
        points={[
          new Vector2(0, gitLevels[i].y - 23),
          new Vector2(0, gitLevels[i + 1].y + 23),
        ]}
        stroke={accent}
        lineWidth={1.5}
        opacity={0.6}
        endArrow
        arrowSize={7}
      />
    );
  }

  gitGroup().add(
    <Txt
      text="Git"
      fontSize={20}
      fontFamily="'JetBrains Mono', monospace"
      fontWeight={700}
      fill={accent}
      y={-220}
    />
  );

  // knowing cascade (right)
  const knowingGroup = createRef<Node>();
  view.add(<Node ref={knowingGroup} x={260} opacity={0} />);

  const knowingLevels = [
    {label: 'source file', y: 160, color: fresh},
    {label: 'edge hashes', y: 53, color: fresh},
    {label: 'subgraph', y: -53, color: fresh},
    {label: 'snapshot root', y: -160, color: fresh},
  ];

  const knowingBoxes = createRefArray<Rect>();
  for (const lvl of knowingLevels) {
    knowingGroup().add(
      <Rect
        ref={knowingBoxes}
        y={lvl.y}
        width={170}
        height={46}
        radius={6}
        fill={codeBg}
        stroke={lvl.color}
        lineWidth={1.5}
      >
        <Txt
          text={lvl.label}
          fontSize={13}
          fontFamily="'JetBrains Mono', monospace"
          fill={text}
        />
      </Rect>
    );
  }

  const knowingEdges = createRefArray<Line>();
  for (let i = 0; i < 3; i++) {
    knowingGroup().add(
      <Line
        ref={knowingEdges}
        points={[
          new Vector2(0, knowingLevels[i].y - 23),
          new Vector2(0, knowingLevels[i + 1].y + 23),
        ]}
        stroke={fresh}
        lineWidth={1.5}
        opacity={0.6}
        endArrow
        arrowSize={7}
      />
    );
  }

  knowingGroup().add(
    <Txt
      text="knowing"
      fontSize={20}
      fontFamily="'JetBrains Mono', monospace"
      fontWeight={700}
      fill={fresh}
      y={-220}
    />
  );

  // Center connector
  const sameModel = createRef<Txt>();
  view.add(
    <Txt
      ref={sameModel}
      text="="
      fontSize={40}
      fontFamily="'JetBrains Mono', monospace"
      fill={muted}
      opacity={0}
    />
  );

  // --- Animate Act 2 ---
  yield* subtitle().text('Git solved versioned state for files.', 0.5);
  yield* gitGroup().opacity(1, 0.6);
  yield* waitFor(1.5);

  yield* subtitle().text('Same mechanism. Different unit of storage.', 0.5);
  yield* all(
    knowingGroup().opacity(1, 0.6),
    sameModel().opacity(1, 0.4),
  );
  yield* waitFor(3.0);

  // Fade out Act 2
  yield* all(
    gitGroup().opacity(0, 0.5),
    knowingGroup().opacity(0, 0.5),
    sameModel().opacity(0, 0.5),
    subtitle().text('', 0.3),
  );
  yield* waitFor(0.5);

  // ============================================================
  // ACT 3: THE MECHANISM
  // ============================================================

  yield* actLabel().text('THE MECHANISM', 0.3);

  const graphGroup = createRef<Node>();
  view.add(<Node ref={graphGroup} opacity={0} />);

  // Files (bottom row)
  const fileData = [
    {name: 'context.go', code: 'func ForTask(desc string)', hash: 'a3f2c8', x: -320, y: 210},
    {name: 'store.go', code: '', hash: 'b7e1d4', x: 0, y: 210},
    {name: 'ranking.go', code: '', hash: 'c9d4f1', x: 320, y: 210},
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
        width={250}
        height={80}
        radius={8}
        fill={codeBg}
        stroke={dim}
        lineWidth={1.5}
      >
        <Txt
          text={f.name}
          fontSize={11}
          fontFamily="'JetBrains Mono', monospace"
          fill={muted}
          y={-24}
        />
        <Txt
          ref={gFileCode}
          text={f.code}
          fontSize={12}
          fontFamily="'JetBrains Mono', monospace"
          fill={text}
          y={0}
        />
        <Txt
          ref={gFileHashes}
          text={f.hash}
          fontSize={11}
          fontFamily="'JetBrains Mono', monospace"
          fill={muted}
          y={24}
        />
      </Rect>
    );
  }

  // Symbols (middle)
  const symData = [
    {name: 'ForTask', x: -380, y: 40, fi: 0},
    {name: 'Rank', x: -200, y: 40, fi: 0},
    {name: 'Edges', x: -30, y: 40, fi: 1},
    {name: 'Store', x: 140, y: 40, fi: 1},
    {name: 'HITS', x: 330, y: 40, fi: 2},
  ];

  const gSymbols = createRefArray<Circle>();
  for (const s of symData) {
    graphGroup().add(
      <Circle
        ref={gSymbols}
        x={s.x}
        y={s.y}
        width={56}
        height={56}
        fill={'#252535'}
      >
        <Txt
          text={s.name}
          fontSize={11}
          fontFamily="'JetBrains Mono', monospace"
          fontWeight={700}
          fill={'#ffffff'}
        />
      </Circle>
    );
  }

  // Root
  const gRoot = createRef<Rect>();
  const gRootHash = createRef<Txt>();
  graphGroup().add(
    <Rect
      ref={gRoot}
      x={0}
      y={-150}
      width={190}
      height={56}
      radius={8}
      fill={codeBg}
      stroke={dim}
      lineWidth={2}
    >
      <Txt
        text="snapshot"
        fontSize={10}
        fontFamily="'JetBrains Mono', monospace"
        fill={muted}
        y={-12}
      />
      <Txt
        ref={gRootHash}
        text="f8a2e7"
        fontSize={15}
        fontFamily="'JetBrains Mono', monospace"
        fontWeight={700}
        fill={accent}
        y={10}
      />
    </Rect>
  );

  // File -> Symbol edges
  const gFEdges = createRefArray<Line>();
  for (const s of symData) {
    graphGroup().add(
      <Line
        ref={gFEdges}
        points={[
          new Vector2(fileData[s.fi].x, fileData[s.fi].y - 40),
          new Vector2(s.x, s.y + 28),
        ]}
        stroke={edgeIdle}
        lineWidth={1.5}
      />
    );
  }

  // Symbol -> Root edges
  const gSEdges = createRefArray<Line>();
  for (const s of symData) {
    graphGroup().add(
      <Line
        ref={gSEdges}
        points={[
          new Vector2(s.x, s.y - 28),
          new Vector2(0, -122),
        ]}
        stroke={edgeIdle}
        lineWidth={1.5}
      />
    );
  }

  // --- Animate Act 3 ---
  yield* graphGroup().opacity(1, 0.7);
  yield* subtitle().text('The graph at rest. Every hash is current.', 0.4);
  yield* waitFor(1.8);

  // Edit
  yield* subtitle().text('A developer adds a parameter.', 0.4);
  yield* waitFor(0.4);
  yield* gFileCode[0].text('func ForTask(desc string, n int)', 0.6);
  yield* waitFor(0.5);

  // Hash recompute with rolling effect
  yield* subtitle().text('Content changed. Hash recomputes.', 0.4);
  yield* gFileRects[0].stroke(pulse, 0.2);
  yield* gFileHashes[0].fill(pulse, 0.15);

  // Rolling hash digits
  const hashSteps = ['a3f...', 'd2...', 'd2f1..', 'd2f1a9'];
  for (const step of hashSteps) {
    yield* gFileHashes[0].text(step, 0.08);
    yield* waitFor(0.08);
  }
  yield* all(
    gFileHashes[0].fill(fresh, 0.3),
    gFileRects[0].stroke(fresh, 0.3),
  );
  yield* waitFor(0.5);

  // Dim unaffected (the stillness IS the point)
  yield* subtitle().text('Only the affected path propagates.', 0.4);
  yield* all(
    gFileRects[1].opacity(0.25, 0.5),
    gFileRects[2].opacity(0.25, 0.5),
    gSymbols[2].opacity(0.25, 0.5),
    gSymbols[3].opacity(0.25, 0.5),
    gSymbols[4].opacity(0.25, 0.5),
    gFEdges[2].opacity(0.1, 0.5),
    gFEdges[3].opacity(0.1, 0.5),
    gFEdges[4].opacity(0.1, 0.5),
    gSEdges[2].opacity(0.1, 0.5),
    gSEdges[3].opacity(0.1, 0.5),
    gSEdges[4].opacity(0.1, 0.5),
  );

  yield* waitFor(0.3);

  // Cascade: file -> symbols (accelerating)
  yield* all(
    gFEdges[0].stroke(pulse, 0.25),
    gFEdges[1].stroke(pulse, 0.25),
  );
  yield* all(
    gSymbols[0].fill(pulse, 0.2),
    gSymbols[1].fill(pulse, 0.2),
  );

  // Cascade: symbols -> root (faster, accelerating feel)
  yield* all(
    gSEdges[0].stroke(pulse, 0.2),
    gSEdges[1].stroke(pulse, 0.2),
  );
  yield* gRoot().stroke(pulse, 0.2);
  yield* waitFor(0.3);

  // Settle green
  yield* all(
    gFEdges[0].stroke(fresh, 0.4),
    gFEdges[1].stroke(fresh, 0.4),
    gSymbols[0].fill(fresh, 0.4),
    gSymbols[1].fill(fresh, 0.4),
    gSEdges[0].stroke(fresh, 0.4),
    gSEdges[1].stroke(fresh, 0.4),
    gRoot().stroke(fresh, 0.4),
  );

  // Root hash rolls
  yield* gRootHash().text('...', 0.08);
  yield* waitFor(0.1);
  yield* gRootHash().text('2b8c41', 0.12);
  yield* waitFor(0.8);

  yield* subtitle().text('2 edges re-extracted. 5 untouched. No full re-index.', 0.5);
  yield* waitFor(3.0);

  // Fade Act 3
  yield* graphGroup().opacity(0, 0.6);
  yield* subtitle().text('', 0.3);
  yield* waitFor(0.4);

  // ============================================================
  // ACT 4: THE PAYOFF
  // ============================================================

  yield* actLabel().text('THE RESULT', 0.3);

  const payoffGroup = createRef<Node>();
  view.add(<Node ref={payoffGroup} opacity={0} />);

  // The single call
  payoffGroup().add(
    <Txt
      text='> context_for_task("refactor auth middleware")'
      fontSize={16}
      fontFamily="'JetBrains Mono', monospace"
      fill={accent}
      fontWeight={600}
      y={-120}
    />
  );

  // Ranked results with scores
  const results = [
    {sym: 'ForTask', score: '0.94', tag: 'authority', color: fresh},
    {sym: 'RankSymbols', score: '0.87', tag: 'hub', color: fresh},
    {sym: 'AuthMiddleware', score: '0.82', tag: 'blast: 12', color: accent},
    {sym: 'SessionStore', score: '0.71', tag: 'runtime ✓', color: accent},
    {sym: 'HandleLogin', score: '0.65', tag: 'feedback +3', color: accent},
  ];

  const resultTxts = createRefArray<Txt>();
  for (let i = 0; i < results.length; i++) {
    const r = results[i];
    payoffGroup().add(
      <Txt
        ref={resultTxts}
        text={`  ${r.sym.padEnd(18)} ${r.score}  (${r.tag})`}
        fontSize={14}
        fontFamily="'JetBrains Mono', monospace"
        fill={text}
        y={-60 + i * 32}
        opacity={0}
      />
    );
  }

  // Comparison bar
  const compBefore = createRef<Rect>();
  const compAfter = createRef<Rect>();
  const compBeforeLabel = createRef<Txt>();
  const compAfterLabel = createRef<Txt>();

  payoffGroup().add(
    <Node y={180}>
      <Txt text="before" fontSize={11} fontFamily="'JetBrains Mono', monospace" fill={muted} x={-200} y={-18} />
      <Rect ref={compBefore} x={-200} width={0} height={24} radius={4} fill={pulse} opacity={0.8} />
      <Txt ref={compBeforeLabel} text="" fontSize={11} fontFamily="'JetBrains Mono', monospace" fill={text} x={-200} y={0} />

      <Txt text="after" fontSize={11} fontFamily="'JetBrains Mono', monospace" fill={muted} x={-200} y={28} />
      <Rect ref={compAfter} x={-200} width={0} height={24} radius={4} fill={fresh} opacity={0.8} y={46} />
      <Txt ref={compAfterLabel} text="" fontSize={11} fontFamily="'JetBrains Mono', monospace" fill={text} x={-200} y={46} />
    </Node>
  );

  // --- Animate Act 4 ---
  yield* payoffGroup().opacity(1, 0.5);
  yield* subtitle().text('One call. Ranked by structure, runtime, and feedback.', 0.5);
  yield* waitFor(0.5);

  // Results cascade in
  for (let i = 0; i < results.length; i++) {
    yield* resultTxts[i].opacity(1, 0.15);
    yield* waitFor(0.12);
  }
  yield* waitFor(0.8);

  // Comparison bars animate
  yield* subtitle().text('', 0.3);
  yield* all(
    compBefore().width(300, 0.8),
    compBeforeLabel().text('7 calls · 10,430 tokens', 0),
  );
  yield* all(
    compAfter().width(70, 0.8),
    compAfterLabel().text('1 call · 2,400 tokens', 0),
  );
  yield* waitFor(2.5);

  // Fade out payoff
  yield* all(
    payoffGroup().opacity(0, 0.6),
    subtitle().text('', 0.3),
    actLabel().text('', 0.3),
  );
  yield* waitFor(0.5);

  // ============================================================
  // CLOSING TAGLINE
  // ============================================================

  const tagline1 = createRef<Txt>();
  const tagline2 = createRef<Txt>();

  view.add(
    <Txt
      ref={tagline1}
      text="Identity is content."
      fontSize={32}
      fontFamily="'JetBrains Mono', monospace"
      fontWeight={700}
      fill={text}
      y={-20}
      opacity={0}
    />
  );
  view.add(
    <Txt
      ref={tagline2}
      text="Incremental updates are structural."
      fontSize={22}
      fontFamily="'JetBrains Mono', monospace"
      fontWeight={300}
      fill={muted}
      y={25}
      opacity={0}
    />
  );

  yield* tagline1().opacity(1, 0.8);
  yield* waitFor(0.4);
  yield* tagline2().opacity(1, 0.6);
  yield* waitFor(4.0);
});
