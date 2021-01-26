import React, { useState, useEffect, useRef } from "react";
import { useDispatch, useSelector } from "react-redux";
import { withShortcut } from "react-keybind";
import clsx from "clsx";
import ProfilerFlameGraph from "./ProfilerFlameGraph";
import ProfilerHeader from "./ProfilerHeader";
import ProfilerTable from "./ProfilerTable";
import { fetchJSON } from "../redux/actions";
import { buildRenderURL } from "../util/update_requests";
import { colorBasedOnPackageName, colorGreyscale } from "../util/color";
import {
  numberWithCommas,
  formatPercent,
  DurationFormater,
} from "../util/format";

const PX_PER_LEVEL = 18;
const COLLAPSE_THRESHOLD = 5;
const LABEL_THRESHOLD = 20;
const HIGHLIGHT_NODE_COLOR = "#48CE73"; // green
const GAP = 0.5;

const Profiler = withShortcut(({ shortcut }) => {
  const state = useSelector((state) => state);
  const from = state.from;
  const until = state.until;
  const labels = state.labels;
  const renderURL = buildRenderURL({ from, until, labels });
  const dispatch = useDispatch();
  const tooltipRef = useRef(null);
  const canvasRef = useRef(null);
  const flamebearer = useSelector((state) => state.flamebearer);
  let canvas = canvasRef.current;
  let ctx = canvasRef.current && canvasRef.current.getContext("2d");
  const levels = flamebearer && flamebearer.levels;
  const [viewState, setViewState] = useState({
    highlightStyle: { display: "none" },
    tooltipStyle: { display: "none" },
    resetStyle: { visibility: "hidden" },
    sortBy: "self",
    sortByDirection: "desc",
    view: "both",
  });
  const prevViewState = usePrevious(viewState);

  const [canvasData, setCanvasData] = useState({
    topLevel: 0,
    selectedLevel: 0,
    rangeMin: 0,
    rangeMax: 1,
    query: "",
  });

  useEffect(() => {
    dispatch(fetchJSON(renderURL));
  }, [from, until]);

  useEffect(() => {
    // compare previous state, viewState before deciding to render
    console.log(prevViewState, viewState);

    renderCanvas(flamebearer, canvasData);
  }, [state, viewState]);

  useEffect(() => {
    if (shortcut) {
      shortcut.registerShortcut(
        reset,
        ["escape"],
        "Reset",
        "Reset Flamegraph View"
      );
    }
  }, []);

  useEffect(() => {
    window.addEventListener("resize", resizeHandler);
    window.addEventListener("focus", focusHandler);
  }, [resizeHandler, focusHandler]);

  const graphWidth = canvas && (canvas.width = canvas.clientWidth);
  const pxPerTick =
    flamebearer &&
    graphWidth /
      flamebearer.numTicks /
      (canvasData.rangeMax - canvasData.rangeMin);
  const tickToX = (i) =>
    (i - flamebearer.numTicks * canvasData.rangeMin) * pxPerTick;

  if (!flamebearer) return <></>;

  return (
    <div className="canvas-renderer">
      <div className="canvas-container">
        <ProfilerHeader
          view={viewState.view}
          handleSearchChange={handleSearchChange}
          reset={reset}
          updateView={updateView}
          resetStyle={viewState.resetStyle}
        />
        <div className="flamegraph-container panes-wrapper">
          <ProfilerTable
            flamebearer={flamebearer}
            viewState={viewState}
            sortByDirection={viewState.sortByDirection}
            sortBy={viewState.sortBy}
            setViewState={setViewState}
          />
          <ProfilerFlameGraph
            view={viewState.view ?? "both"}
            canvasRef={canvasRef}
            clickHandler={() => {}}
            mouseMoveHandler={() => {}}
            mouseOutHandler={() => {}}
          />
        </div>
        <div
          className={clsx("no-data-message", {
            visible: flamebearer && flamebearer.numTicks === 0,
          })}
        >
          <span>
            No profiling data available for this application / time range.
          </span>
        </div>
      </div>
      <div className="flamegraph-highlight" style={viewState.highlightStyle} />
      <div
        className="flamegraph-tooltip"
        ref={tooltipRef}
        style={viewState.tooltipStyle}
      >
        <div className="flamegraph-tooltip-name">{viewState.tooltipTitle}</div>
        <div>{viewState.tooltipSubtitle}</div>
      </div>
    </div>
  );

  function rect(ctx, x, y, w, h, radius) {
    return ctx.rect(x, y, w, h);
  }

  function roundRect(ctx, x, y, w, h, radius) {
    if (radius >= w / 2) {
      return rect(ctx, x, y, w, h, radius);
    }
    radius = Math.min(w / 2, radius);
    const r = x + w;
    const b = y + h;
    ctx.beginPath();
    ctx.moveTo(x + radius, y);
    ctx.lineTo(r - radius, y);
    ctx.quadraticCurveTo(r, y, r, y + radius);
    ctx.lineTo(r, y + h - radius);
    ctx.quadraticCurveTo(r, b, r - radius, b);
    ctx.lineTo(x + radius, b);
    ctx.quadraticCurveTo(x, b, x, b - radius);
    ctx.lineTo(x, y + radius);
    ctx.quadraticCurveTo(x, y, x + radius, y);
  }

  function updateResetStyle(selectedLevel, setViewState) {
    // const emptyQuery = this.query === "";
    setViewState({
      ...viewState,
      resetStyle: { visibility: selectedLevel === 0 ? "hidden" : "visible" },
    });
  }

  function updateZoom(i, j) {
    const { selectedLevel } = canvasData;
    if (!levels) return;
    if (!Number.isNaN(i) && !Number.isNaN(j)) {
      setCanvasData({
        selectedLevel: i,
        topLevel: 0,
        rangeMin: levels[i][j] / flamebearer.numTicks,
        rangeMax: (levels[i][j] + levels[i][j + 1]) / flamebearer.numTicks,
        ...canvasData,
      });
    } else {
      setCanvasData({
        selectedLevel: 0,
        topLevel: 0,
        rangeMin: 0,
        rangeMax: 1,
        canvasData,
      });
    }
    updateResetStyle(selectedLevel, setViewState);
  }

  function renderCanvas(flamebearer, canvasData) {
    if (!flamebearer || !canvas) {
      return;
    }
    const { topLevel, selectedLevel, rangeMin, rangeMax, query } = canvasData;
    const { names, levels, numTicks, sampleRate, spyName } = flamebearer;

    canvas.height = PX_PER_LEVEL * (levels.length - topLevel);
    canvas.style.height = `${canvas.height}px`;

    if (devicePixelRatio > 1) {
      canvas.width *= 2;
      canvas.height *= 2;
      ctx.scale(2, 2);
    }

    ctx.textBaseline = "middle";
    ctx.font =
      '400 12px system-ui, -apple-system, "Segoe UI", "Roboto", "Ubuntu", "Cantarell", "Noto Sans", sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol", "Noto Color Emoji"';

    const df = new DurationFormater(numTicks / sampleRate);
    // i = level
    for (let i = 0; i < levels.length - topLevel; i++) {
      const level = levels[topLevel + i];

      for (let j = 0; j < level.length; j += 4) {
        // j = 0: x start of bar
        // j = 1: width of bar
        // j = 2: position in the main index

        const barIndex = level[j];
        const x = tickToX(barIndex);
        const y = i * PX_PER_LEVEL;
        let numBarTicks = level[j + 1];

        // For this particular bar, there is a match
        const queryExists = query.length > 0;
        const nodeIsInQuery =
          (query && names[level[j + 3]].indexOf(query) >= 0) || false;
        // merge very small blocks into big "collapsed" ones for performance
        const collapsed = numBarTicks * pxPerTick <= COLLAPSE_THRESHOLD;

        // const collapsed = false;
        if (collapsed) {
          while (
            j < level.length - 3 &&
            barIndex + numBarTicks === level[j + 3] &&
            level[j + 4] * pxPerTick <= COLLAPSE_THRESHOLD &&
            nodeIsInQuery ===
              ((query && names[level[j + 5]].indexOf(query) >= 0) || false)
          ) {
            j += 4;
            numBarTicks += level[j + 1];
          }
        }
        // ticks are samples
        const sw = numBarTicks * pxPerTick - (collapsed ? 0 : GAP);
        const sh = PX_PER_LEVEL - GAP;

        // if (x < -1 || x + sw > this.graphWidth + 1 || sw < HIDE_THRESHOLD) continue;

        ctx.beginPath();
        rect(ctx, x, y, sw, sh, 3);

        const ratio = numBarTicks / numTicks;

        const a = selectedLevel > i ? 0.33 : 1;

        let nodeColor;
        if (collapsed) {
          nodeColor = colorGreyscale(200, 0.66);
        } else if (queryExists && nodeIsInQuery) {
          nodeColor = HIGHLIGHT_NODE_COLOR;
        } else if (queryExists && !nodeIsInQuery) {
          nodeColor = colorGreyscale(200, 0.66);
        } else {
          nodeColor = colorBasedOnPackageName(
            getPackageNameFromStackTrace(spyName, names[level[j + 3]]),
            a
          );
        }

        ctx.fillStyle = nodeColor;
        ctx.fill();

        if (!collapsed && sw >= LABEL_THRESHOLD) {
          const percent = formatPercent(ratio);
          const name = `${names[level[j + 3]]} (${percent}, ${df.format(
            numBarTicks / sampleRate
          )})`;

          ctx.save();
          ctx.clip();
          ctx.fillStyle = "black";
          ctx.fillText(name, Math.round(Math.max(x, 0) + 3), y + sh / 2);
          ctx.restore();
        }
      }
    }
  }

  function getPackageNameFromStackTrace(spyName, stackTrace) {
    const regexpLookup = {
      pyspy: /^(?<packageName>(.*\/)*)(?<filename>.*\.py+)(?<line_info>.*)$/,
      rbspy: /^(?<func>.+? - )?(?<packageName>(.*\/)*)(?<filename>.*)(?<line_info>.*)$/,
      gospy: /^(?<packageName>(.*\/)*)(?<filename>.*)(?<line_info>.*)$/,
      default: /^(?<packageName>(.*\/)*)(?<filename>.*)(?<line_info>.*)$/,
    };

    if (stackTrace.length == 0) {
      return stackTrace;
    }
    const regexp = regexpLookup[spyName] || regexpLookup.default;
    const fullStackGroups = stackTrace.match(regexp);
    if (fullStackGroups) {
      return fullStackGroups.groups.packageName;
    }
    return stackTrace;
  }

  // binary search of a block in a stack level
  function binarySearchLevel(x, level, tickToX) {
    let i = 0;
    let j = level.length - 4;
    while (i <= j) {
      const m = 4 * ((i / 4 + j / 4) >> 1);
      const x0 = tickToX(level[m]);
      const x1 = tickToX(level[m] + level[m + 1]);
      if (x0 <= x && x1 >= x) {
        return x1 - x0 > COLLAPSE_THRESHOLD ? m : -1;
      }
      if (x0 > x) {
        j = m - 4;
      } else {
        i = m + 4;
      }
    }
    return -1;
  }

  function handleSearchChange(e) {
    setCanvasData({ ...canvasData, query: e.target.value });
    updateResetStyle(canvasData.selectedLevel, setViewState);
  }

  function reset() {
    updateZoom(0, 0);
    renderCanvas(flamebearer, canvasData);
  }

  function xyToBar(x, y) {
    const i = Math.floor(y / PX_PER_LEVEL) + canvasData.topLevel;
    if (i >= 0 && i < levels.length) {
      const j = binarySearchLevel(x, levels[i], tickToX);
      return { i, j };
    }
    return { i: 0, j: 0 };
  }

  function clickHandler(e) {
    const { i, j } = xyToBar(e.nativeEvent.offsetX, e.nativeEvent.offsetY);
    if (j === -1) return;
    updateZoom(i, j);
    renderCanvas(flamebearer, canvasData);
    mouseOutHandler();
  }

  function resizeHandler() {
    // this is here to debounce resize events (see: https://css-tricks.com/debouncing-throttling-explained-examples/)
    //   because rendering is expensive
    // clearTimeout(resizeFinish);
    // resizeFinish = setTimeout(() => renderCanvas(flamebearer, canvasData), 100);
  }

  function focusHandler() {
    renderCanvas(flamebearer, canvasData);
  }

  function updateView(newView) {
    setViewState({
      ...viewState,
      view: newView,
    });
    // console.log('render-canvas');
    setTimeout(renderCanvas(flamebearer, canvasData), 1000);
  }

  function mouseMoveHandler(e) {
    const { i, j } = xyToBar(e.nativeEvent.offsetX, e.nativeEvent.offsetY);

    if (
      j === -1 ||
      e.nativeEvent.offsetX < 0 ||
      e.nativeEvent.offsetX > graphWidth
    ) {
      mouseOutHandler();
      return;
    }

    canvas.style.cursor = "pointer";

    const level = levels[i];
    const x = Math.max(tickToX(level[j]), 0);
    const y = (i - canvasData.topLevel) * PX_PER_LEVEL;
    const sw = Math.min(tickToX(level[j] + level[j + 1]) - x, graphWidth);

    const tooltipEl = tooltipRef.current;
    const numBarTicks = level[j + 1];
    const percent = formatPercent(numBarTicks / flamebearer.numTicks);

    // a little hacky but this is here so that we can get tooltipWidth after text is updated.
    const tooltipTitle = flamebearer.names[level[j + 3]];
    tooltipEl.children[0].innerText = tooltipTitle;
    const tooltipWidth = tooltipEl.clientWidth;

    const df = new DurationFormater(
      flamebearer.numTicks / flamebearer.sampleRate
    );

    setViewState({
      ...viewState,
      highlightStyle: {
        display: "block",
        left: `${canvas.offsetLeft + x}px`,
        top: `${canvas.offsetTop + y}px`,
        width: `${sw}px`,
        height: `${PX_PER_LEVEL}px`,
      },
      tooltipStyle: {
        display: "block",
        left: `${
          Math.min(
            canvas.offsetLeft + e.nativeEvent.offsetX + 15 + tooltipWidth,
            canvas.offsetLeft + graphWidth
          ) - tooltipWidth
        }px`,
        top: `${canvas.offsetTop + e.nativeEvent.offsetY + 12}px`,
      },
      tooltipTitle,
      tooltipSubtitle: `${percent}, ${numberWithCommas(
        numBarTicks
      )} samples, ${df.format(numBarTicks / flamebearer.sampleRate)}`,
    });
  }

  function mouseOutHandler() {
    canvas.style.cursor = "";
    setViewState({
      ...viewState,
      highlightStyle: {
        display: "none",
      },
      tooltipStyle: {
        display: "none",
      },
    });
  }
});

function usePrevious(value) {
  const ref = useRef();
  useEffect(() => {
    ref.current = value;
  });
  return ref.current;
}

export default withShortcut(Profiler);
