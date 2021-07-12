/* eslint-disable */
// ISC License

// Copyright (c) 2018, Mapbox

// Permission to use, copy, modify, and/or distribute this software for any purpose
// with or without fee is hereby granted, provided that the above copyright notice
// and this permission notice appear in all copies.

// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH
// REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND
// FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT,
// INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS
// OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER
// TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF
// THIS SOFTWARE.

// This component is based on flamebearer project
//   https://github.com/mapbox/flamebearer

import React from "react";
import { connect } from "react-redux";

import clsx from "clsx";

import { bindActionCreators } from "redux";

import { withShortcut } from "react-keybind";

import { buildRenderURL } from "../util/updateRequests";
import {
  numberWithCommas,
  formatPercent,
  getPackageNameFromStackTrace,
  getFormatter,
} from "../util/format";
import { colorBasedOnPackageName, colorGreyscale } from "../util/color";
import TimelineChartWrapper from "./TimelineChartWrapper";
import ProfilerTable from "./ProfilerTable";
import ProfilerHeader from "./ProfilerHeader";
import { deltaDiff } from "../util/flamebearer";

import ExportData from "./ExportData";


const PX_PER_LEVEL = 18;
const COLLAPSE_THRESHOLD = 5;
const LABEL_THRESHOLD = 20;
const HIGHLIGHT_NODE_COLOR = "#48CE73"; // green
const GAP = 0.5;

const unitsToFlamegraphTitle = {
  "objects": "amount of objects in RAM per function",
  "bytes": "amount of RAM per function",
  "samples": "CPU time per function",
}

class FlameGraphRenderer extends React.Component {
  constructor() {
    super();
    this.state = {
      highlightStyle: { display: "none" },
      tooltipStyle: { display: "none" },
      resetStyle: { visibility: "hidden" },
      sortBy: "self",
      sortByDirection: "desc",
      view: "both",
      flamebearer: null,
    };
    this.canvasRef = React.createRef();
    this.highlightRef = React.createRef();
    this.tooltipRef = React.createRef();
    this.currentJSONController = null;
  }

  componentDidMount() {
    this.canvas = this.canvasRef.current;
    this.ctx = this.canvas.getContext("2d");
    this.topLevel = 0; // Todo: could be a constant
    this.selectedLevel = 0;
    this.rangeMin = 0;
    this.rangeMax = 1;
    this.query = "";

    window.addEventListener("resize", this.resizeHandler);
    window.addEventListener("focus", this.focusHandler);

    if (this.props.shortcut) {
      this.props.shortcut.registerShortcut(
        this.reset,
        ["escape"],
        "Reset",
        "Reset Flamegraph View"
      );
    }
  }

  componentDidUpdate(prevProps, prevState) {
    if (this.getParamsFromRenderURL(this.props.renderURL).name != this.getParamsFromRenderURL(prevProps.renderURL).name ||
      prevProps.from != this.props.from ||
      prevProps.until != this.props.until ||
      prevProps.maxNodes != this.props.maxNodes ||
      prevProps.refreshToken != this.props.refreshToken ||
      prevProps[`${this.props.viewSide}From`] != this.props[`${this.props.viewSide}From`] ||
      prevProps[`${this.props.viewSide}Until`] != this.props[`${this.props.viewSide}Until`] ||
      prevProps.flamebearer != this.props.flamebearer    
    ) {
      if(this.props.viewSide === 'left' || this.props.viewSide === 'right') {
        this.updateFlameBearerData(this.props[`${this.props.viewSide}RenderURL`], this.props.flamebearer)
      } else {
        this.updateFlameBearerData(this.props.renderURL, this.props.flamebearer)
      }
    }

    if (
      this.state.flamebearer &&
      prevState.flamebearer != this.state.flamebearer
    ) {
      this.updateData();
    }
  }

  updateFlameBearerData(url, flamebearer) {
    if (this.currentJSONController) {
      this.currentJSONController.abort();
    }
    this.currentJSONController = new AbortController();
    if (!flamebearer) {
      fetch(`${url}&format=json`, { signal: this.currentJSONController.signal })
        .then((response) => response.json())
        .then((data) => {
          let flamebearer = data.flamebearer;
          deltaDiff(flamebearer.levels);
  
          this.setState({
            flamebearer: flamebearer
          }, () => {
            this.updateData();
          })
        })
        .finally();
    } else {
      this.setState({
        flamebearer: flamebearer
      }, () => {
        this.updateData();
      })
    }
  }

  getParamsFromRenderURL(inputURL) {
    let urlParamsRegexp = /(.*render\?)(?<urlParams>(.*))/
    let paramsString = inputURL.match(urlParamsRegexp);

    let params = new URLSearchParams(paramsString.groups.urlParams);
    let paramsObj = this.paramsToObject(params);

    return paramsObj
  }

  paramsToObject(entries) {
    const result = {}
    for(const [key, value] of entries) { // each 'entry' is a [key, value] tupple
      result[key] = value;
    }
    return result;
  }

  rect(ctx, x, y, w, h, radius) {
    return ctx.rect(x, y, w, h);
  }

  roundRect(ctx, x, y, w, h, radius) {
    if (radius >= w / 2) {
      return this.rect(ctx, x, y, w, h, radius);
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

  updateZoom(i, j) {
    if (!Number.isNaN(i) && !Number.isNaN(j)) {
      this.selectedLevel = i;
      this.topLevel = 0;
      this.rangeMin = this.state.levels[i][j] / this.state.numTicks;
      this.rangeMax =
        (this.state.levels[i][j] + this.state.levels[i][j + 1]) / this.state.numTicks;
    } else {
      this.selectedLevel = 0;
      this.topLevel = 0;
      this.rangeMin = 0;
      this.rangeMax = 1;
    }
    this.updateResetStyle();
  }

  updateData = () => {
    const { names, levels, numTicks, sampleRate, units } = this.state.flamebearer;
    this.setState({
      names: names,
      levels: levels,
      numTicks: numTicks,
      sampleRate: sampleRate,
      units: units,
    }, () => {
      this.renderCanvas();
    });
  };

  // binary search of a block in a stack level
  binarySearchLevel(x, level, tickToX) {
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

  updateResetStyle = () => {
    // const emptyQuery = this.query === "";
    const topLevelSelected = this.selectedLevel === 0;
    this.setState({
      resetStyle: { visibility: topLevelSelected ? "hidden" : "visible" },
    });
  };

  handleSearchChange = (e) => {
    this.query = e.target.value;
    this.updateResetStyle();
    this.renderCanvas();
  };

  reset = () => {
    this.updateZoom(0, 0);
    this.renderCanvas();
  };

  xyToBar = (x, y) => {
    const i = Math.floor(y / PX_PER_LEVEL) + this.topLevel;
    if (i >= 0 && i < this.state.levels.length) {
      const j = this.binarySearchLevel(x, this.state.levels[i], this.tickToX);
      return { i, j };
    }
    return { i: 0, j: 0 };
  };

  clickHandler = (e) => {
    const { i, j } = this.xyToBar(e.nativeEvent.offsetX, e.nativeEvent.offsetY);
    if (j === -1) return;

    this.updateZoom(i, j);
    this.renderCanvas();
    this.mouseOutHandler();
  };

  resizeHandler = () => {
    // this is here to debounce resize events (see: https://css-tricks.com/debouncing-throttling-explained-examples/)
    //   because rendering is expensive
    clearTimeout(this.resizeFinish);
    this.resizeFinish = setTimeout(this.renderCanvas, 100);
  };

  focusHandler = () => {
    this.renderCanvas();
  };

  tickToX = (i) => (i - this.state.numTicks * this.rangeMin) * this.pxPerTick;

  updateView = (newView) => {
    this.setState({
      view: newView,
    });
    setTimeout(this.renderCanvas, 0);
  };

  createFormatter = () => {
    return getFormatter(this.state.numTicks, this.state.sampleRate, this.state.units);
  }

  renderCanvas = () => {
    if (!this.state.names) {
      return;
    }

    const { names, levels, numTicks, sampleRate, units } = this.state;

    this.graphWidth = this.canvas.width = this.canvas.clientWidth;
    this.pxPerTick =
      this.graphWidth / numTicks / (this.rangeMax - this.rangeMin);
    this.canvas.height = PX_PER_LEVEL * (levels.length - this.topLevel);
    this.canvas.style.height = `${this.canvas.height}px`;
    this.canvas.style.cursor = "pointer";

    if (devicePixelRatio > 1) {
      this.canvas.width *= 2;
      this.canvas.height *= 2;
      this.ctx.scale(2, 2);
    }

    this.ctx.textBaseline = "middle";
    this.ctx.font =
      '400 12px system-ui, -apple-system, "Segoe UI", "Roboto", "Ubuntu", "Cantarell", "Noto Sans", sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol", "Noto Color Emoji"';

    this.formatter = this.createFormatter();
    // i = level
    for (let i = 0; i < levels.length - this.topLevel; i++) {
      const level = levels[this.topLevel + i];
      for (let j = 0; j < level.length; j += 4) {
        // j = 0: x start of bar
        // j = 1: width of bar
        // j = 2: position in the main index

        const barIndex = level[j];
        const x = this.tickToX(barIndex);
        const y = i * PX_PER_LEVEL;
        let numBarTicks = level[j + 1];

        // For this particular bar, there is a match
        const queryExists = this.query.length > 0;
        const nodeIsInQuery =
          (this.query && names[level[j + 3]].indexOf(this.query) >= 0) || false;
        // merge very small blocks into big "collapsed" ones for performance
        const collapsed = numBarTicks * this.pxPerTick <= COLLAPSE_THRESHOLD;

        // const collapsed = false;
        if (collapsed) {
          while (
            j < level.length - 3 &&
            barIndex + numBarTicks === level[j + 3] &&
            level[j + 4] * this.pxPerTick <= COLLAPSE_THRESHOLD &&
            nodeIsInQuery ===
              ((this.query && names[level[j + 5]].indexOf(this.query) >= 0) ||
                false)
          ) {
            j += 4;
            numBarTicks += level[j + 1];
          }
        }
        // ticks are samples
        const sw = numBarTicks * this.pxPerTick - (collapsed ? 0 : GAP);
        const sh = PX_PER_LEVEL - GAP;

        // if (x < -1 || x + sw > this.graphWidth + 1 || sw < HIDE_THRESHOLD) continue;

        this.ctx.beginPath();
        this.rect(this.ctx, x, y, sw, sh, 3);

        const ratio = numBarTicks / numTicks;

        const a = this.selectedLevel > i ? 0.33 : 1;

        const { spyName } = this.state.flamebearer;

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

        this.ctx.fillStyle = nodeColor;
        this.ctx.fill();

        if (!collapsed && sw >= LABEL_THRESHOLD) {
          const percent = formatPercent(ratio);
          const name = `${names[level[j + 3]]} (${percent}, ${this.formatter.format(numBarTicks, sampleRate)})`;

          this.ctx.save();
          this.ctx.clip();
          this.ctx.fillStyle = "black";
          this.ctx.fillText(name, Math.round(Math.max(x, 0) + 3), y + sh / 2);
          this.ctx.restore();
        }
      }
    }
  };

  mouseMoveHandler = (e) => {
    const { i, j } = this.xyToBar(e.nativeEvent.offsetX, e.nativeEvent.offsetY);

    if (
      j === -1 ||
      e.nativeEvent.offsetX < 0 ||
      e.nativeEvent.offsetX > this.graphWidth
    ) {
      this.mouseOutHandler();
      return;
    }

    const level = this.state.levels[i];
    const x = Math.max(this.tickToX(level[j]), 0);
    const y = (i - this.topLevel) * PX_PER_LEVEL;
    const sw = Math.min(
      this.tickToX(level[j] + level[j + 1]) - x,
      this.graphWidth
    );

    const highlightEl = this.highlightRef.current;
    const tooltipEl = this.tooltipRef.current;
    const numBarTicks = level[j + 1];
    const percent = formatPercent(numBarTicks / this.state.numTicks);
    const tooltipTitle = this.state.names[level[j + 3]];

    // Before you change all of this to React consider performance implications.
    // Doing this with setState leads to significant lag.
    // See this issue https://github.com/pyroscope-io/pyroscope/issues/205
    //   and this PR https://github.com/pyroscope-io/pyroscope/pull/266 for more info.
    highlightEl.style.opacity = 1;
    highlightEl.style.left = `${this.canvas.offsetLeft + x}px`;
    highlightEl.style.top = `${this.canvas.offsetTop + y}px`;
    highlightEl.style.width = `${sw}px`;
    highlightEl.style.height = `${PX_PER_LEVEL}px`;

    tooltipEl.style.opacity = 1;
    tooltipEl.style.left = `${e.clientX+12}px`;
    tooltipEl.style.top = `${e.clientY+12}px`;

    tooltipEl.children[0].innerText = tooltipTitle;
    tooltipEl.children[1].innerText = `${percent}, ${numberWithCommas(
      numBarTicks
    )} samples, ${this.formatter.format(numBarTicks, this.state.sampleRate)}`;
  };

  mouseOutHandler = () => {
    this.highlightRef.current.style.opacity = "0";
    this.tooltipRef.current.style.opacity = "0";
  };

  updateSortBy = (newSortBy) => {
    let dir = this.state.sortByDirection;
    if (this.state.sortBy == newSortBy) {
      dir = dir == "asc" ? "desc" : "asc";
    } else {
      dir = "desc";
    }
    this.setState({
      sortBy: newSortBy,
      sortByDirection: dir,
    });
  };

  render = () => {
    // This is necessary because the order switches depending on single vs comparison view
    let tablePane = (
      <div
        key={'table-pane'}
        className={clsx("pane", { hidden: this.state.view === "icicle", "vertical-orientation": this.props.viewType === "double" })}
      >
        <ProfilerTable
          flamebearer={this.state.flamebearer}
          sortByDirection={this.state.sortByDirection}
          sortBy={this.state.sortBy}
          updateSortBy={this.updateSortBy}
          view={this.state.view}
        />
      </div>
    )

    let flameGraphPane = (
      <div
        key={'flamegraph-pane'}
        className={clsx("pane", { hidden: this.state.view === "table", "vertical-orientation": this.props.viewType === "double" })}
      >
        <div className='flamegraph-header'>
          <span></span>
          <span>Frame width represents {unitsToFlamegraphTitle[this.state.units]}</span>
          <ExportData flameCanvas={this.canvasRef}/>
        </div>
        <canvas
          className="flamegraph-canvas"
          height="0"
          ref={this.canvasRef}
          onClick={this.clickHandler}
          onMouseMove={this.mouseMoveHandler}
          onMouseOut={this.mouseOutHandler}
        />
      </div>
    )

    let panes = this.props.viewType === "double" ?
      [flameGraphPane, tablePane]:
      [tablePane, flameGraphPane]

    const flotData = this.props.timeline
      ? [this.props.timeline.map((x) => [x[0], x[1] === 0 ? null : x[1] - 1])]
      : [];

    let instructionsText = this.props.viewType === "double" ? `Select ${this.props.viewSide} time range` : null;
    let instructionsClassName = this.props.viewType === "double" ? `${this.props.viewSide}-instructions` : null;

    return (
      <div className={clsx("canvas-renderer", { "double": this.props.viewType === "double" })}>

        <div className="canvas-container">
          <ProfilerHeader
            view={this.state.view}
            handleSearchChange={this.handleSearchChange}
            reset={this.reset}
            updateView={this.updateView}
            resetStyle={this.state.resetStyle}
          />
          <div className={`${instructionsClassName}-wrapper`}>
            <span className={`${instructionsClassName}-text`}>{instructionsText}</span>
          </div>
          {
            this.props.viewType === "double" ?
              <TimelineChartWrapper
                key={`timeline-chart-${this.props.viewSide}`}
                id={`timeline-chart-${this.props.viewSide}`}
                viewSide={this.props.viewSide}
              /> :
              null
          }
          <div className={clsx("flamegraph-container panes-wrapper", { "vertical-orientation": this.props.viewType === "double" })}>
            {
              panes.map((pane) => (
                pane
              ))
            }
            {/* { tablePane }
            { flameGraphPane } */}
          </div>
          <div
            className={clsx("no-data-message", {
              visible:
                this.state.flamebearer && this.state.flamebearer.numTicks === 0,
            })}
          >
            <span>
              No profiling data available for this application / time range.
            </span>
          </div>
        </div>
        <div
          className="flamegraph-highlight"
          ref={this.highlightRef}
        />
        <div
          className="flamegraph-tooltip"
          ref={this.tooltipRef}
        >
          <div className="flamegraph-tooltip-name"></div>
          <div></div>
        </div>
      </div>
    )
}
}

const mapStateToProps = (state) => ({
  ...state,
  renderURL: buildRenderURL(state),
  leftRenderURL: buildRenderURL(state, state.leftFrom, state.leftUntil),
  rightRenderURL: buildRenderURL(state, state.rightFrom, state.rightUntil),
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    { },
    dispatch
  ),
});

export default connect(
  mapStateToProps,
  mapDispatchToProps
)(withShortcut(FlameGraphRenderer));
