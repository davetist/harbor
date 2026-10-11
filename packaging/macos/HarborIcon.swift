import AppKit

enum HarborIcon {
    // Vector geometry from web/public/harbor-logo.svg. Clip the waves to the
    // circle so the same mark stays sharp at every app and menu-bar size.
    static func draw(in bounds: CGRect, color: NSColor, cropped: Bool = false) {
        guard let context = NSGraphicsContext.current?.cgContext else { return }
        context.saveGState()
        context.translateBy(x: bounds.minX, y: bounds.maxY)
        let extent: CGFloat = cropped ? 49 : 56
        context.scaleBy(x: bounds.width / extent, y: -bounds.height / extent)
        if cropped { context.translateBy(x: -3.5, y: -3.5) }
        context.addEllipse(in: CGRect(x: 3.5, y: 3.5, width: 49, height: 49))
        context.clip()

        let waves = CGMutablePath()
        waves.move(to: CGPoint(x: 0, y: 25.737))
        waves.addLine(to: CGPoint(x: 8.878, y: 25.737))
        waves.addCurve(to: CGPoint(x: 20.542, y: 21.822), control1: CGPoint(x: 11.787, y: 25.737), control2: CGPoint(x: 14.23, y: 21.822))
        waves.addCurve(to: CGPoint(x: 37.581, y: 28.346), control1: CGPoint(x: 28.735, y: 21.822), control2: CGPoint(x: 31.276, y: 28.346))
        waves.addCurve(to: CGPoint(x: 46.61, y: 25.737), control1: CGPoint(x: 41.562, y: 28.346), control2: CGPoint(x: 43.816, y: 25.737))
        waves.addLine(to: CGPoint(x: 56, y: 25.737))
        waves.addLine(to: CGPoint(x: 56, y: 0))
        waves.addLine(to: .zero)
        waves.closeSubpath()
        waves.move(to: CGPoint(x: 0, y: 30.225))
        waves.addLine(to: CGPoint(x: 9.608, y: 30.225))
        waves.addCurve(to: CGPoint(x: 20.229, y: 27.041), control1: CGPoint(x: 12.596, y: 30.225), control2: CGPoint(x: 15.789, y: 27.041))
        waves.addCurve(to: CGPoint(x: 37.242, y: 33.252), control1: CGPoint(x: 26.644, y: 27.041), control2: CGPoint(x: 29.277, y: 33.252))
        waves.addCurve(to: CGPoint(x: 49.585, y: 30.173), control1: CGPoint(x: 43.374, y: 33.252), control2: CGPoint(x: 43.374, y: 30.173))
        waves.addLine(to: CGPoint(x: 56, y: 30.173))
        waves.addLine(to: CGPoint(x: 56, y: 56))
        waves.addLine(to: CGPoint(x: 0, y: 56))
        waves.closeSubpath()
        context.setFillColor(color.cgColor)
        context.addPath(waves)
        context.fillPath()
        context.restoreGState()
    }

    static func menuBarImage() -> NSImage {
        let image = NSImage(size: NSSize(width: 18, height: 18), flipped: false) { bounds in
            draw(in: bounds, color: .black, cropped: true)
            return true
        }
        image.isTemplate = true
        image.accessibilityDescription = "Harbor"
        return image
    }
}
