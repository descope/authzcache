package errors

import ce "github.com/descope/common/pkg/common/errors"

const errorServiceID = "17"

var Builder = ce.NewErrorTypeBuilder(errorServiceID)

// Service 2XXX
var UnknownNodeType = Builder.NewExportedBadRequestErrorType("2001", "Node type is not known")
var UnknownNodeExpressionType = Builder.NewExportedBadRequestErrorType("2002", "Node expression type is not known")
var UnknownRelationDefinition = Builder.NewExportedBadRequestErrorType("2003", "Relation Definition is not known")
var UnknownNamespace = Builder.NewExportedBadRequestErrorType("2004", "Namespace is not known")
var UnknownProject = Builder.NewExportedBadRequestErrorType("2005", "Unknown project id")
var CircularRelationDefinitions = Builder.NewExportedBadRequestErrorType("2006", "Circular relation definitions detected")
var CircularRelation = Builder.NewExportedBadRequestErrorType("2007", "Circular relation detected")
var SchemaDoesNotExist = Builder.NewExportedBadRequestErrorType("2004", "Schema does not exist")
var SchemaParsingError = Builder.NewExportedBadRequestErrorType("2005", "Error parsing schema")
var FGAUnknownType = Builder.NewExportedBadRequestErrorType("2006", "Unknown type")
var FGAUnknownRelation = Builder.NewExportedBadRequestErrorType("2007", "Unknown relation")
var FGAParseError = Builder.NewExportedBadRequestErrorType("2008", "Error parsing FGA")
var FGAWrongTypeForRelation = Builder.NewExportedBadRequestErrorType("2009", "Wrong type for relation")
var FGADuplicateType = Builder.NewExportedBadRequestErrorType("2010", "Duplicate type")
var FGADuplicateRelation = Builder.NewExportedBadRequestErrorType("2011", "Duplicate relation")
var FGADuplicatePermission = Builder.NewExportedBadRequestErrorType("2012", "Duplicate permission")
var FGADuplicateName = Builder.NewExportedBadRequestErrorType("2013", "Duplicate name")

// Repo 3XXX
var NamespaceExists = Builder.NewInternalErrorType("3001")
var NamespaceSave = Builder.NewInternalErrorType("3002")
var RelationDefinitionExists = Builder.NewInternalErrorType("3003")
var RelationDefinitionSave = Builder.NewInternalErrorType("3004")
var RelationSave = Builder.NewInternalErrorType("3005")
var RelationDelete = Builder.NewInternalErrorType("3006")
var SchemaSave = Builder.NewInternalErrorType("3007")
var SchemaLoad = Builder.NewInternalErrorType("3008")
var RelationSearch = Builder.NewInternalErrorType("3009")
var GetModified = Builder.NewInternalErrorType("3010")
var RelationChangePartitionDelete = Builder.NewInternalErrorType("3011")

// Remote AuthZ client 4XXX
var MissingClientConfigParameter = Builder.NewInternalErrorType("4000")
var RemoteClientForbiddenMethod = Builder.NewInternalErrorType("4001")
