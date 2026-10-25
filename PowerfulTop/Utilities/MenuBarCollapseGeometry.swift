import CoreGraphics

// Collapse geometry adapted from MenubarHide
// (https://github.com/junior-rj/menubar-hide, MIT License, © Sparrow Serviços e Soluções em Informática Ltda).

/// How long the separator and its spacers get so macOS 27 moves the items left of them into
/// its own « overflow menu.
///
/// macOS 27 draws every status item into one menu bar window and drops an item whose length
/// reaches a per-display cliff instead of clamping it, so the classic 10 000 pt separator simply
/// vanishes. Below the cliff the bar overflows from the left until the rest fits, so separator
/// plus spacers must span the widest status area while each one stays under the narrowest cliff
/// (one length applies to every display's copy of the bar).
///
/// Cliffs measured on 27.0: half the width without a notch; on a notched display about 0.79 of
/// the area right of the notch, whatever the frontmost app's menus take.
enum MenuBarCollapseGeometry {
    struct Display: Equatable {
        let width: CGFloat
        /// Room for status items: right of the notch when there is one, the full width otherwise.
        let statusWidth: CGFloat

        init(width: CGFloat, statusWidth: CGFloat? = nil) {
            self.width = width
            self.statusWidth = statusWidth ?? width
        }
    }

    /// Fixed so every launch registers the same autosave names: a name first seen on a later
    /// launch lands leftmost, outside the separator block.
    static let spacerCount = 6
    static let minimumUnit: CGFloat = 200
    /// The measured cliffs are not exact.
    static let cliffMargin: CGFloat = 64
    /// Stays under the ~0.79 measured on notched displays.
    static let notchedCliffFactor: CGFloat = 0.75

    static func cliff(of display: Display) -> CGFloat {
        display.statusWidth < display.width
            ? display.statusWidth * notchedCliffFactor
            : display.width / 2
    }

    /// Length for the separator and each active spacer.
    static func unitLength(displays: [Display]) -> CGFloat {
        guard let lowest = displays.map(cliff(of:)).min() else { return minimumUnit }
        return max(minimumUnit, (lowest - cliffMargin).rounded(.down))
    }

    /// Fewest spacers that cover the widest status area: extra ones would overflow themselves
    /// and show up as blank rows in the « menu.
    static func activeSpacers(unit: CGFloat, displays: [Display]) -> Int {
        guard unit > 0, let widest = displays.map(\.statusWidth).max() else { return 0 }
        let needed = Int((widest / unit).rounded(.up)) - 1
        return min(max(needed, 0), spacerCount)
    }
}
