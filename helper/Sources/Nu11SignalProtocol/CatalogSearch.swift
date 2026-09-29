// The pure argument handling of the `searchCatalog` command, free of
// MusicKit so it can be unit tested.
import Foundation

/// The validated arguments of `searchCatalog`: a term that is not blank
/// and a per-type result limit within what the catalog serves.
public struct CatalogSearchQuery: Equatable {
    /// The catalog search endpoint returns at most 25 results per type.
    public static let maxLimit = 25
    /// The suggestions endpoint returns at most 10 terms.
    public static let maxSuggestions = 10
    /// Top results are a short list above the per-type sections, as in
    /// Apple Music; the catalog would return `limit` of them, mostly
    /// repeating those sections.
    public static let maxTopResults = 6

    /// The term as sent; only a blank one is rejected.
    public let term: String
    /// Results per type (artists, albums, songs, playlists): `limit`, clamped to
    /// 1...maxLimit; maxLimit when absent.
    public let limit: Int

    /// How many suggestion terms to ask for.
    public var suggestionLimit: Int { min(limit, Self.maxSuggestions) }

    /// How many top results to keep.
    public var topResultLimit: Int { min(limit, Self.maxTopResults) }

    public init(_ request: Request) throws {
        guard let term = request.string("term"),
              !term.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            throw ArgumentError(description: "\(request.cmd) requires a non-empty \"term\"")
        }
        var limit = Self.maxLimit
        if request.args["limit"] != nil {
            guard let value = request.int("limit") else {
                throw ArgumentError(description: "\"limit\" must be an integer")
            }
            limit = min(max(value, 1), Self.maxLimit)
        }
        self.term = term
        self.limit = limit
    }
}
