package aws

import (
	"bytes"
	"fmt"
	"log"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/kinesisanalyticsv2"
	"github.com/hashicorp/terraform-plugin-sdk/helper/hashcode"
	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"
	"github.com/terraform-providers/terraform-provider-aws/aws/internal/keyvaluetags"
)

var validateKinesisAnalyticsV2Runtime = validation.StringInSlice([]string{
	kinesisanalyticsv2.RuntimeEnvironmentSql10,
	kinesisanalyticsv2.RuntimeEnvironmentFlink16,
	kinesisanalyticsv2.RuntimeEnvironmentFlink18,
}, false)

func resourceAwsKinesisAnalyticsV2Application() *schema.Resource {
	return &schema.Resource{
		Create: resourceAwsKinesisAnalyticsV2ApplicationCreate,
		Read:   resourceAwsKinesisAnalyticsV2ApplicationRead,
		Update: resourceAwsKinesisAnalyticsV2ApplicationUpdate,
		Delete: resourceAwsKinesisAnalyticsV2ApplicationDelete,

		Importer: &schema.ResourceImporter{
			State: func(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
				arns := strings.Split(d.Id(), ":")
				name := strings.Replace(arns[len(arns)-1], "application/", "", 1)
				d.Set("name", name)
				return []*schema.ResourceData{d}, nil
			},
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"arn": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"service_execution_role": {
				Type:     schema.TypeString,
				Required: true,
			},

			"runtime": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validateKinesisAnalyticsV2Runtime,
			},

			"s3_bucket": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"s3_object": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"code": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"code_content_type": {
				Type:     schema.TypeString,
				Required: true,
			},

			"create_timestamp": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"description": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"last_update_timestamp": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"status": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"version": {
				Type:     schema.TypeInt,
				Computed: true,
			},

			"cloudwatch_logging_options": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeString,
							Computed: true,
						},

						"log_stream_arn": {
							Type:         schema.TypeString,
							Required:     true,
							ValidateFunc: validateArn,
						},
					},
				},
			},

			// Flink only
			"property_groups": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"property_group_id": {
							Type:     schema.TypeString,
							Required: true,
						},
						"property_map": {
							Type:     schema.TypeMap,
							Required: true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
					},
				},
			},
			"snapshots_enabled": {
				Type:     schema.TypeBool,
				Optional: true,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					snapshotsEnabled := strconv.FormatBool(d.Get(k).(bool))
					return snapshotsEnabled == old
				},
			},
			"flink_application_configuration": {
				Type:          schema.TypeList,
				Optional:      true,
				MaxItems:      1,
				ConflictsWith: []string{"sql_application_configuration"},
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"checkpoint_configuration": {
							Type:     schema.TypeSet,
							Optional: true,
							Computed: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"checkpoint_interval": {
										Type:     schema.TypeInt,
										Optional: true,
									},
									"checkpointing_enabled": {
										Type:     schema.TypeBool,
										Optional: true,
									},
									"configuration_type": {
										Type:         schema.TypeString,
										Optional:     true,
										ValidateFunc: validateKinesisAnalyticsConfigurationType,
									},
									"min_pause_between_checkpoints": {
										Type:     schema.TypeInt,
										Optional: true,
									},
								},
							},
						},
						"monitoring_configuration": {
							Type:     schema.TypeSet,
							Optional: true,
							Computed: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"configuration_type": {
										Type:         schema.TypeString,
										Optional:     true,
										ValidateFunc: validateKinesisAnalyticsConfigurationType,
									},
									"log_level": {
										Type:         schema.TypeString,
										Required:     true,
										ValidateFunc: validateLogLevel,
									},
									"metrics_level": {
										Type:         schema.TypeString,
										Required:     true,
										ValidateFunc: validateKineisAnalyticsMetricsLevel,
									},
								},
							},
						},
						"parallelism_configuration": {
							Type:     schema.TypeSet,
							Optional: true,
							Computed: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"autoscaling_enabled": {
										Type:     schema.TypeBool,
										Optional: true,
									},
									"configuration_type": {
										Type:         schema.TypeString,
										Optional:     true,
										ValidateFunc: validateKinesisAnalyticsConfigurationType,
									},
									"parallelism": {
										Type:     schema.TypeInt,
										Required: true,
									},
									"parallelism_per_kpu": {
										Type:     schema.TypeInt,
										Required: true,
									},
								},
							},
						},
					},
				},
			},

			"sql_application_configuration": {
				Type:          schema.TypeList,
				Optional:      true,
				MaxItems:      1,
				ConflictsWith: []string{"flink_application_configuration"},
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"inputs": {
							Type:     schema.TypeList,
							Optional: true,
							MaxItems: 1,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"id": {
										Type:     schema.TypeString,
										Computed: true,
									},

									"kinesis_firehose": {
										Type:     schema.TypeList,
										Optional: true,
										MaxItems: 1,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"resource_arn": {
													Type:         schema.TypeString,
													Required:     true,
													ValidateFunc: validateArn,
												},
											},
										},
									},

									"kinesis_stream": {
										Type:     schema.TypeList,
										Optional: true,
										MaxItems: 1,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"resource_arn": {
													Type:         schema.TypeString,
													Required:     true,
													ValidateFunc: validateArn,
												},
											},
										},
									},

									"name_prefix": {
										Type:     schema.TypeString,
										Required: true,
									},

									"parallelism": {
										Type:     schema.TypeList,
										Optional: true,
										MaxItems: 1,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"count": {
													Type:     schema.TypeInt,
													Required: true,
												},
											},
										},
									},

									"processing_configuration": {
										Type:     schema.TypeList,
										Optional: true,
										MaxItems: 1,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"lambda": {
													Type:     schema.TypeList,
													Required: true,
													MaxItems: 1,
													Elem: &schema.Resource{
														Schema: map[string]*schema.Schema{
															"resource_arn": {
																Type:         schema.TypeString,
																Required:     true,
																ValidateFunc: validateArn,
															},
														},
													},
												},
											},
										},
									},

									"schema": {
										Type:     schema.TypeList,
										Required: true,
										MaxItems: 1,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"record_columns": {
													Type:     schema.TypeList,
													Required: true,
													Elem: &schema.Resource{
														Schema: map[string]*schema.Schema{
															"mapping": {
																Type:     schema.TypeString,
																Optional: true,
															},

															"name": {
																Type:     schema.TypeString,
																Required: true,
															},

															"sql_type": {
																Type:     schema.TypeString,
																Required: true,
															},
														},
													},
												},

												"record_encoding": {
													Type:     schema.TypeString,
													Optional: true,
												},

												"record_format": {
													Type:     schema.TypeList,
													Required: true,
													MaxItems: 1,
													Elem: &schema.Resource{
														Schema: map[string]*schema.Schema{
															"mapping_parameters": {
																Type:     schema.TypeList,
																Optional: true,
																MaxItems: 1,
																Elem: &schema.Resource{
																	Schema: map[string]*schema.Schema{
																		"csv": {
																			Type:     schema.TypeList,
																			Optional: true,
																			MaxItems: 1,
																			Elem: &schema.Resource{
																				Schema: map[string]*schema.Schema{
																					"record_column_delimiter": {
																						Type:     schema.TypeString,
																						Required: true,
																					},

																					"record_row_delimiter": {
																						Type:     schema.TypeString,
																						Required: true,
																					},
																				},
																			},
																		},

																		"json": {
																			Type:     schema.TypeList,
																			Optional: true,
																			MaxItems: 1,
																			Elem: &schema.Resource{
																				Schema: map[string]*schema.Schema{
																					"record_row_path": {
																						Type:     schema.TypeString,
																						Required: true,
																					},
																				},
																			},
																		},
																	},
																},
															},

															"record_format_type": {
																Type:     schema.TypeString,
																Computed: true,
															},
														},
													},
												},
											},
										},
									},

									"starting_position_configuration": {
										Type:     schema.TypeList,
										Computed: true,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"starting_position": {
													Type:     schema.TypeString,
													Computed: true,
												},
											},
										},
									},

									"stream_names": {
										Type:     schema.TypeSet,
										Computed: true,
										Elem:     &schema.Schema{Type: schema.TypeString},
									},
								},
							},
						},

						"outputs": {
							Type:     schema.TypeList,
							Optional: true,
							MaxItems: 3,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"id": {
										Type:     schema.TypeString,
										Computed: true,
									},

									"kinesis_firehose": {
										Type:     schema.TypeList,
										Optional: true,
										MaxItems: 1,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"resource_arn": {
													Type:         schema.TypeString,
													Required:     true,
													ValidateFunc: validateArn,
												},
											},
										},
									},

									"kinesis_stream": {
										Type:     schema.TypeList,
										Optional: true,
										MaxItems: 1,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"resource_arn": {
													Type:         schema.TypeString,
													Required:     true,
													ValidateFunc: validateArn,
												},
											},
										},
									},

									"lambda": {
										Type:     schema.TypeList,
										Optional: true,
										MaxItems: 1,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"resource_arn": {
													Type:         schema.TypeString,
													Required:     true,
													ValidateFunc: validateArn,
												},
											},
										},
									},

									"name": {
										Type:     schema.TypeString,
										Required: true,
									},

									"schema": {
										Type:     schema.TypeList,
										Required: true,
										MaxItems: 1,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"record_format_type": {
													Type:     schema.TypeString,
													Optional: true,
													ValidateFunc: validation.StringInSlice([]string{
														kinesisanalyticsv2.RecordFormatTypeCsv,
														kinesisanalyticsv2.RecordFormatTypeJson,
													}, false),
												},
											},
										},
									},
								},
							},
						},

						"reference_data_sources": {
							Type:     schema.TypeList,
							Optional: true,
							MaxItems: 1,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"id": {
										Type:     schema.TypeString,
										Computed: true,
									},

									"s3": {
										Type:     schema.TypeList,
										Required: true,
										MaxItems: 1,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"bucket_arn": {
													Type:         schema.TypeString,
													Required:     true,
													ValidateFunc: validateArn,
												},

												"file_key": {
													Type:     schema.TypeString,
													Required: true,
												},
												"object_version": {
													Type:     schema.TypeString,
													Optional: true,
												},
											},
										},
									},

									"schema": {
										Type:     schema.TypeList,
										Required: true,
										MaxItems: 1,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"record_columns": {
													Type:     schema.TypeList,
													Required: true,
													Elem: &schema.Resource{
														Schema: map[string]*schema.Schema{
															"mapping": {
																Type:     schema.TypeString,
																Optional: true,
															},

															"name": {
																Type:     schema.TypeString,
																Required: true,
															},

															"sql_type": {
																Type:     schema.TypeString,
																Required: true,
															},
														},
													},
												},

												"record_encoding": {
													Type:     schema.TypeString,
													Optional: true,
												},

												"record_format": {
													Type:     schema.TypeList,
													Required: true,
													MaxItems: 1,
													Elem: &schema.Resource{
														Schema: map[string]*schema.Schema{
															"mapping_parameters": {
																Type:     schema.TypeList,
																Optional: true,
																MaxItems: 1,
																Elem: &schema.Resource{
																	Schema: map[string]*schema.Schema{
																		"csv": {
																			Type:     schema.TypeList,
																			Optional: true,
																			MaxItems: 1,
																			Elem: &schema.Resource{
																				Schema: map[string]*schema.Schema{
																					"record_column_delimiter": {
																						Type:     schema.TypeString,
																						Required: true,
																					},

																					"record_row_delimiter": {
																						Type:     schema.TypeString,
																						Required: true,
																					},
																				},
																			},
																		},

																		"json": {
																			Type:     schema.TypeList,
																			Optional: true,
																			MaxItems: 1,
																			Elem: &schema.Resource{
																				Schema: map[string]*schema.Schema{
																					"record_row_path": {
																						Type:     schema.TypeString,
																						Required: true,
																					},
																				},
																			},
																		},
																	},
																},
															},

															"record_format_type": {
																Type:     schema.TypeString,
																Computed: true,
															},
														},
													},
												},
											},
										},
									},

									"table_name": {
										Type:     schema.TypeString,
										Required: true,
									},
								},
							},
						},
					},
				},
			},
			"tags": tagsSchema(),
		},
	}
}

func resourceAwsKinesisAnalyticsV2ApplicationCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).kinesisanalyticsv2conn
	name := d.Get("name").(string)
	serviceExecutionRole := d.Get("service_execution_role").(string)
	runtime := d.Get("runtime").(string)
	s3Bucket := d.Get("s3_bucket").(string)
	s3Object := d.Get("s3_object").(string)
	textCode := d.Get("code").(string)
	codeContentType := d.Get("code_content_type").(string)

	var sqlApplicationConfiguration *kinesisanalyticsv2.SqlApplicationConfiguration
	var flinkApplicationConfiguration *kinesisanalyticsv2.FlinkApplicationConfiguration
	switch {
	case strings.HasPrefix(runtime, "SQL"):
		sqlConfig, ok := d.GetOk("sql_application_configuration")
		if !ok {
			break
		}
		sc := sqlConfig.([]interface{})[0]
		if sc != nil {
			sqlApplicationConfiguration = expandKinesisAnalayticsSqlApplicationConfiguration(sc.(map[string]interface{}))
		}

	case strings.HasPrefix(runtime, "FLINK"):
		flinkConfig, ok := d.GetOk("flink_application_configuration")
		if !ok {
			break
		}
		fc := flinkConfig.([]interface{})[0]
		if fc != nil {
			flinkApplicationConfiguration = expandKinesisAnalyticsFlinkApplicationConfiguration(fc.(map[string]interface{}))
		}
	}

	var contentType *string
	switch codeContentType {
	case "zip":
		contentType = aws.String(kinesisanalyticsv2.CodeContentTypeZipfile)
	case "plain_text":
		contentType = aws.String(kinesisanalyticsv2.CodeContentTypePlaintext)
	}

	var s3ContentLocation *kinesisanalyticsv2.S3ContentLocation
	if s3Bucket != "" && s3Object != "" {
		s3ContentLocation = &kinesisanalyticsv2.S3ContentLocation{
			BucketARN: aws.String(s3Bucket),
			FileKey:   aws.String(s3Object),
		}
		if version, ok := d.GetOk("object_version"); ok {
			s3ContentLocation.ObjectVersion = aws.String(version.(string))
		}
	}

	var environmentProperties *kinesisanalyticsv2.EnvironmentProperties
	if v, ok := d.GetOk("property_groups"); ok {
		m := v.([]interface{})
		environmentProperties = &kinesisanalyticsv2.EnvironmentProperties{
			PropertyGroups: expandPropertyGroups(m),
		}
	}
	var snapshotConfig *kinesisanalyticsv2.ApplicationSnapshotConfiguration
	if v, ok := d.GetOk("snapshots_enabled"); ok {
		snapshotsEnabled, _ := v.(bool)
		snapshotConfig = &kinesisanalyticsv2.ApplicationSnapshotConfiguration{
			SnapshotsEnabled: aws.Bool(snapshotsEnabled),
		}
	}

	var textContent *string
	if textCode != "" {
		textContent = aws.String(textCode)
	}

	createOpts := &kinesisanalyticsv2.CreateApplicationInput{
		RuntimeEnvironment:   aws.String(runtime),
		ApplicationName:      aws.String(name),
		ServiceExecutionRole: aws.String(serviceExecutionRole),
		ApplicationConfiguration: &kinesisanalyticsv2.ApplicationConfiguration{
			SqlApplicationConfiguration:      sqlApplicationConfiguration,
			FlinkApplicationConfiguration:    flinkApplicationConfiguration,
			EnvironmentProperties:            environmentProperties,
			ApplicationSnapshotConfiguration: snapshotConfig,
			ApplicationCodeConfiguration: &kinesisanalyticsv2.ApplicationCodeConfiguration{
				CodeContent: &kinesisanalyticsv2.CodeContent{
					S3ContentLocation: s3ContentLocation,
					TextContent:       textContent,
				},
				CodeContentType: contentType,
			},
		},
	}

	if v, ok := d.GetOk("code"); ok && v.(string) != "" {
		createOpts.ApplicationConfiguration.ApplicationCodeConfiguration.CodeContent.TextContent = aws.String(v.(string))
	}

	if v, ok := d.GetOk("cloudwatch_logging_options"); ok {
		clo := v.([]interface{})[0].(map[string]interface{})
		cloudwatchLoggingOption := expandKinesisAnalyticsV2CloudwatchLoggingOption(clo)
		createOpts.CloudWatchLoggingOptions = []*kinesisanalyticsv2.CloudWatchLoggingOption{cloudwatchLoggingOption}
	}

	if v := d.Get("tags").(map[string]interface{}); len(v) > 0 {
		createOpts.Tags = keyvaluetags.New(v).IgnoreAws().Kinesisanalyticsv2Tags()
	}

	// Retry for IAM eventual consistency
	err := resource.Retry(1*time.Minute, func() *resource.RetryError {
		output, err := conn.CreateApplication(createOpts)
		if err != nil {
			// Kinesis Stream: https://github.com/terraform-providers/terraform-provider-aws/issues/7032
			if isAWSErr(err, kinesisanalyticsv2.ErrCodeInvalidArgumentException, "Kinesis Analytics service doesn't have sufficient privileges") {
				return resource.RetryableError(err)
			}
			// Kinesis Firehose: https://github.com/terraform-providers/terraform-provider-aws/issues/7394
			if isAWSErr(err, kinesisanalyticsv2.ErrCodeInvalidArgumentException, "Kinesis Analytics doesn't have sufficient privileges") {
				return resource.RetryableError(err)
			}
			// InvalidArgumentException: Given IAM role arn : arn:aws:iam::123456789012:role/xxx does not provide Invoke permissions on the Lambda resource : arn:aws:lambda:us-west-2:123456789012:function:yyy
			if isAWSErr(err, kinesisanalyticsv2.ErrCodeInvalidArgumentException, "does not provide Invoke permissions on the Lambda resource") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		d.SetId(aws.StringValue(output.ApplicationDetail.ApplicationARN))
		return nil
	})

	if isResourceTimeoutError(err) {
		var output *kinesisanalyticsv2.CreateApplicationOutput
		output, err = conn.CreateApplication(createOpts)
		d.SetId(aws.StringValue(output.ApplicationDetail.ApplicationARN))
	}

	if err != nil {
		return fmt.Errorf("Unable to create Kinesis Analytics application: %s", err)
	}

	return resourceAwsKinesisAnalyticsV2ApplicationUpdate(d, meta)
}

func resourceAwsKinesisAnalyticsV2ApplicationRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).kinesisanalyticsv2conn
	ignoreTagsConfig := meta.(*AWSClient).IgnoreTagsConfig
	name := d.Get("name").(string)

	describeOpts := &kinesisanalyticsv2.DescribeApplicationInput{
		ApplicationName: aws.String(name),
	}
	resp, err := conn.DescribeApplication(describeOpts)
	if isAWSErr(err, kinesisanalyticsv2.ErrCodeResourceNotFoundException, "") {
		log.Printf("[WARN] Kinesis Analytics Application (%s) not found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if err != nil {
		return fmt.Errorf("error reading Kinesis Analytics Application (%s): %s", d.Id(), err)
	}

	arn := aws.StringValue(resp.ApplicationDetail.ApplicationARN)
	d.Set("name", aws.StringValue(resp.ApplicationDetail.ApplicationName))
	d.Set("arn", arn)
	d.Set("service_execution_role", aws.StringValue(resp.ApplicationDetail.ServiceExecutionRole))
	d.Set("runtime", aws.StringValue(resp.ApplicationDetail.RuntimeEnvironment))
	d.Set("code", aws.StringValue(resp.ApplicationDetail.ApplicationConfigurationDescription.ApplicationCodeConfigurationDescription.CodeContentDescription.TextContent))
	var tfContentType string
	contentType := aws.StringValue(resp.ApplicationDetail.ApplicationConfigurationDescription.ApplicationCodeConfigurationDescription.CodeContentType)
	if contentType == kinesisanalyticsv2.CodeContentTypePlaintext {
		tfContentType = "plain_text"
	} else if contentType == kinesisanalyticsv2.CodeContentTypeZipfile {
		tfContentType = "zip"
	}
	d.Set("code_content_type", tfContentType)
	d.Set("create_timestamp", aws.TimeValue(resp.ApplicationDetail.CreateTimestamp).Format(time.RFC3339))
	d.Set("description", aws.StringValue(resp.ApplicationDetail.ApplicationDescription))
	d.Set("last_update_timestamp", aws.TimeValue(resp.ApplicationDetail.LastUpdateTimestamp).Format(time.RFC3339))
	d.Set("status", aws.StringValue(resp.ApplicationDetail.ApplicationStatus))
	d.Set("version", int(aws.Int64Value(resp.ApplicationDetail.ApplicationVersionId)))
	if resp.ApplicationDetail.ApplicationConfigurationDescription.ApplicationCodeConfigurationDescription.CodeContentDescription.S3ApplicationCodeLocationDescription != nil {
		d.Set("s3_bucket", aws.StringValue(resp.ApplicationDetail.ApplicationConfigurationDescription.ApplicationCodeConfigurationDescription.CodeContentDescription.S3ApplicationCodeLocationDescription.BucketARN))
		d.Set("s3_object", aws.StringValue(resp.ApplicationDetail.ApplicationConfigurationDescription.ApplicationCodeConfigurationDescription.CodeContentDescription.S3ApplicationCodeLocationDescription.FileKey))
		if resp.ApplicationDetail.ApplicationConfigurationDescription.ApplicationCodeConfigurationDescription.CodeContentDescription.S3ApplicationCodeLocationDescription.ObjectVersion != nil {
			d.Set("object_version", aws.StringValue(resp.ApplicationDetail.ApplicationConfigurationDescription.ApplicationCodeConfigurationDescription.CodeContentDescription.S3ApplicationCodeLocationDescription.ObjectVersion))
		}
	}

	if err := d.Set("cloudwatch_logging_options", flattenKinesisAnalyticsV2CloudwatchLoggingOptions(resp.ApplicationDetail.CloudWatchLoggingOptionDescriptions)); err != nil {
		return fmt.Errorf("error setting cloudwatch_logging_options: %s", err)
	}

	if resp.ApplicationDetail.ApplicationConfigurationDescription.EnvironmentPropertyDescriptions != nil {
		if err := d.Set("property_groups", flattenKinesisAnalyticsPropertyGroups(resp.ApplicationDetail.ApplicationConfigurationDescription.EnvironmentPropertyDescriptions.PropertyGroupDescriptions)); err != nil {
			return fmt.Errorf("error setting property_groups: %s", err)
		}
	}

	runtime := aws.StringValue(resp.ApplicationDetail.RuntimeEnvironment)
	if runtime == kinesisanalyticsv2.RuntimeEnvironmentSql10 {
		if err := d.Set("sql_application_configuration", flattenSqlApplicationConfigurationDescription(resp.ApplicationDetail.ApplicationConfigurationDescription.SqlApplicationConfigurationDescription)); err != nil {
			return fmt.Errorf("error setting sql_application_configuration: %s", err)
		}
	}
	if runtimeIsFlink(runtime) {
		if err := d.Set("flink_application_configuration", flattenFlinkApplicationConfigurationDescription(resp.ApplicationDetail.ApplicationConfigurationDescription.FlinkApplicationConfigurationDescription)); err != nil {
			return fmt.Errorf("error setting flink_application_configuration: %s", err)
		}
	}
	if resp.ApplicationDetail.ApplicationConfigurationDescription.ApplicationSnapshotConfigurationDescription != nil {
		if err := d.Set("snapshots_enabled", aws.BoolValue(resp.ApplicationDetail.ApplicationConfigurationDescription.ApplicationSnapshotConfigurationDescription.SnapshotsEnabled)); err != nil {
			return fmt.Errorf("error setting snapshots_enabled: %s", err)
		}
	}

	tags, err := keyvaluetags.Kinesisanalyticsv2ListTags(conn, arn)

	if err != nil {
		return fmt.Errorf("error listing tags for Kinesis Analytics Application (%s): %s", arn, err)
	}

	if err := d.Set("tags", tags.IgnoreAws().IgnoreConfig(ignoreTagsConfig).Map()); err != nil {
		return fmt.Errorf("error setting tags: %s", err)
	}

	return nil
}

func resourceAwsKinesisAnalyticsV2ApplicationUpdate(d *schema.ResourceData, meta interface{}) error {
	var version int
	conn := meta.(*AWSClient).kinesisanalyticsv2conn
	name := d.Get("name").(string)

	if v, ok := d.GetOk("version"); ok {
		version = v.(int)
	} else {
		version = 1
	}

	if !d.IsNewResource() {
		updateApplicationOpts := &kinesisanalyticsv2.UpdateApplicationInput{
			ApplicationName:             aws.String(name),
			CurrentApplicationVersionId: aws.Int64(int64(version)),
		}

		applicationUpdate, err := createApplicationV2UpdateOpts(d)
		if err != nil {
			return err
		}

		if !reflect.DeepEqual(applicationUpdate, &kinesisanalyticsv2.UpdateApplicationInput{}) {
			updateApplicationOpts.SetApplicationConfigurationUpdate(applicationUpdate.ApplicationConfigurationUpdate)
			updateApplicationOpts.SetCloudWatchLoggingOptionUpdates(applicationUpdate.CloudWatchLoggingOptionUpdates)
			_, updateErr := conn.UpdateApplication(updateApplicationOpts)
			if updateErr != nil {
				return updateErr
			}
			version = version + 1
		}

		oldLoggingOptions, newLoggingOptions := d.GetChange("cloudwatch_logging_options")
		if len(oldLoggingOptions.([]interface{})) == 0 && len(newLoggingOptions.([]interface{})) > 0 {
			if v, ok := d.GetOk("cloudwatch_logging_options"); ok {
				clo := v.([]interface{})[0].(map[string]interface{})
				cloudwatchLoggingOption := expandKinesisAnalyticsV2CloudwatchLoggingOption(clo)
				addOpts := &kinesisanalyticsv2.AddApplicationCloudWatchLoggingOptionInput{
					ApplicationName:             aws.String(name),
					CurrentApplicationVersionId: aws.Int64(int64(version)),
					CloudWatchLoggingOption:     cloudwatchLoggingOption,
				}
				// Retry for IAM eventual consistency
				err := resource.Retry(1*time.Minute, func() *resource.RetryError {
					_, err := conn.AddApplicationCloudWatchLoggingOption(addOpts)
					if err != nil {
						if isAWSErr(err, kinesisanalyticsv2.ErrCodeInvalidArgumentException, "Kinesis Analytics service doesn't have sufficient privileges") {
							return resource.RetryableError(err)
						}
						return resource.NonRetryableError(err)
					}
					return nil
				})
				if isResourceTimeoutError(err) {
					_, err = conn.AddApplicationCloudWatchLoggingOption(addOpts)
				}

				if err != nil {
					return fmt.Errorf("Unable to add CloudWatch logging options: %s", err)
				}
				version = version + 1
			}
		}
		if d.HasChange("sql_application_configuration") {
			oldConf, newConf := d.GetChange("sql_application_configuration")
			o := oldConf.([]interface{})[0].(map[string]interface{})
			n := newConf.([]interface{})[0].(map[string]interface{})
			oldInputs := o["inputs"].([]interface{})
			oldOutputs := o["outputs"].([]interface{})
			newInputs := n["inputs"].([]interface{})
			newOutputs := n["outputs"].([]interface{})

			if len(oldInputs) == 0 && len(newInputs) > 0 {
				i := newInputs[0].(map[string]interface{})
				input := expandKinesisAnalyticsV2Inputs(i)
				addOpts := &kinesisanalyticsv2.AddApplicationInputInput{
					ApplicationName:             aws.String(name),
					CurrentApplicationVersionId: aws.Int64(int64(version)),
					Input:                       input,
				}
				// Retry for IAM eventual consistency
				err := resource.Retry(1*time.Minute, func() *resource.RetryError {
					_, err := conn.AddApplicationInput(addOpts)
					if err != nil {
						if isAWSErr(err, kinesisanalyticsv2.ErrCodeInvalidArgumentException, "Kinesis Analytics service doesn't have sufficient privileges") {
							return resource.RetryableError(err)
						}
						// InvalidArgumentException: Given IAM role arn : arn:aws:iam::123456789012:role/xxx does not provide Invoke permissions on the Lambda resource : arn:aws:lambda:us-west-2:123456789012:function:yyy
						if isAWSErr(err, kinesisanalyticsv2.ErrCodeInvalidArgumentException, "does not provide Invoke permissions on the Lambda resource") {
							return resource.RetryableError(err)
						}
						return resource.NonRetryableError(err)
					}
					return nil
				})
				if isResourceTimeoutError(err) {
					_, err = conn.AddApplicationInput(addOpts)
				}

				if err != nil {
					return fmt.Errorf("Unable to add application inputs: %s", err)
				}
				version = version + 1
			}
			if len(oldOutputs) == 0 && len(newOutputs) > 0 {
				o := newOutputs[0].(map[string]interface{})
				output := expandKinesisAnalyticsV2Outputs(o)
				addOpts := &kinesisanalyticsv2.AddApplicationOutputInput{
					ApplicationName:             aws.String(name),
					CurrentApplicationVersionId: aws.Int64(int64(version)),
					Output:                      output,
				}
				// Retry for IAM eventual consistency
				err := resource.Retry(1*time.Minute, func() *resource.RetryError {
					_, err := conn.AddApplicationOutput(addOpts)
					if err != nil {
						if isAWSErr(err, kinesisanalyticsv2.ErrCodeInvalidArgumentException, "Kinesis Analytics service doesn't have sufficient privileges") {
							return resource.RetryableError(err)
						}
						// InvalidArgumentException: Given IAM role arn : arn:aws:iam::123456789012:role/xxx does not provide Invoke permissions on the Lambda resource : arn:aws:lambda:us-west-2:123456789012:function:yyy
						if isAWSErr(err, kinesisanalyticsv2.ErrCodeInvalidArgumentException, "does not provide Invoke permissions on the Lambda resource") {
							return resource.RetryableError(err)
						}
						return resource.NonRetryableError(err)
					}
					return nil
				})
				if isResourceTimeoutError(err) {
					_, err = conn.AddApplicationOutput(addOpts)
				}
				if err != nil {
					return fmt.Errorf("Unable to add application outputs: %s", err)
				}
			}
		}
		arn := d.Get("arn").(string)
		if d.HasChange("tags") {
			o, n := d.GetChange("tags")
			if err := keyvaluetags.Kinesisanalyticsv2UpdateTags(conn, arn, o, n); err != nil {
				return fmt.Errorf("error updating Kinesis Analytics Application (%s) tags: %s", arn, err)
			}
		}
	}

	return resourceAwsKinesisAnalyticsV2ApplicationRead(d, meta)
}

func resourceAwsKinesisAnalyticsV2ApplicationDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).kinesisanalyticsv2conn
	name := d.Get("name").(string)
	createTimestamp, parseErr := time.Parse(time.RFC3339, d.Get("create_timestamp").(string))
	if parseErr != nil {
		return parseErr
	}

	log.Printf("[DEBUG] Kinesis Analytics Application destroy: %v", d.Id())
	deleteOpts := &kinesisanalyticsv2.DeleteApplicationInput{
		ApplicationName: aws.String(name),
		CreateTimestamp: aws.Time(createTimestamp),
	}
	_, deleteErr := conn.DeleteApplication(deleteOpts)
	if isAWSErr(deleteErr, kinesisanalyticsv2.ErrCodeResourceNotFoundException, "") {
		return nil
	}
	deleteErr = waitForDeleteKinesisAnalyticsV2Application(conn, d.Id(), d.Timeout(schema.TimeoutDelete))
	if deleteErr != nil {
		return fmt.Errorf("error waiting for deletion of Kinesis Analytics Application (%s): %s", d.Id(), deleteErr)
	}

	log.Printf("[DEBUG] Kinesis Analytics Application deleted: %v", d.Id())
	return nil
}

func expandKinesisAnalyticsV2CloudwatchLoggingOption(clo map[string]interface{}) *kinesisanalyticsv2.CloudWatchLoggingOption {
	cloudwatchLoggingOption := &kinesisanalyticsv2.CloudWatchLoggingOption{
		LogStreamARN: aws.String(clo["log_stream_arn"].(string)),
	}
	return cloudwatchLoggingOption
}

func expandPropertyGroups(i []interface{}) []*kinesisanalyticsv2.PropertyGroup {
	propertyGroups := []*kinesisanalyticsv2.PropertyGroup{}
	for _, v := range i {
		pg := v.(map[string]interface{})
		id := pg["property_group_id"].(string)
		pm := pg["property_map"].(map[string]interface{})

		awsMap := make(map[string]*string, len(pm))
		for k, v := range pm {
			awsMap[k] = aws.String(v.(string))
		}

		propertyGroups = append(propertyGroups, &kinesisanalyticsv2.PropertyGroup{
			PropertyGroupId: aws.String(id),
			PropertyMap:     awsMap,
		})
	}
	return propertyGroups
}

func expandKinesisAnalyticsV2Inputs(i map[string]interface{}) *kinesisanalyticsv2.Input {
	input := &kinesisanalyticsv2.Input{
		NamePrefix: aws.String(i["name_prefix"].(string)),
	}

	if v := i["kinesis_firehose"].([]interface{}); len(v) > 0 {
		kf := v[0].(map[string]interface{})
		kfi := &kinesisanalyticsv2.KinesisFirehoseInput{
			ResourceARN: aws.String(kf["resource_arn"].(string)),
		}
		input.KinesisFirehoseInput = kfi
	}

	if v := i["kinesis_stream"].([]interface{}); len(v) > 0 {
		ks := v[0].(map[string]interface{})
		ksi := &kinesisanalyticsv2.KinesisStreamsInput{
			ResourceARN: aws.String(ks["resource_arn"].(string)),
		}
		input.KinesisStreamsInput = ksi
	}

	if v := i["parallelism"].([]interface{}); len(v) > 0 {
		p := v[0].(map[string]interface{})

		if c, ok := p["count"]; ok {
			ip := &kinesisanalyticsv2.InputParallelism{
				Count: aws.Int64(int64(c.(int))),
			}
			input.InputParallelism = ip
		}
	}

	if v := i["processing_configuration"].([]interface{}); len(v) > 0 {
		pc := v[0].(map[string]interface{})

		if l := pc["lambda"].([]interface{}); len(l) > 0 {
			lp := l[0].(map[string]interface{})
			ipc := &kinesisanalyticsv2.InputProcessingConfiguration{
				InputLambdaProcessor: &kinesisanalyticsv2.InputLambdaProcessor{
					ResourceARN: aws.String(lp["resource_arn"].(string)),
				},
			}
			input.InputProcessingConfiguration = ipc
		}
	}

	if v := i["schema"].([]interface{}); len(v) > 0 {
		vL := v[0].(map[string]interface{})
		ss := expandKinesisAnalyticsV2SourceSchema(vL)
		input.InputSchema = ss
	}

	return input
}

func expandKinesisAnalyticsV2Outputs(o map[string]interface{}) *kinesisanalyticsv2.Output {
	output := &kinesisanalyticsv2.Output{
		Name: aws.String(o["name"].(string)),
	}

	if v := o["kinesis_firehose"].([]interface{}); len(v) > 0 {
		kf := v[0].(map[string]interface{})
		kfo := &kinesisanalyticsv2.KinesisFirehoseOutput{
			ResourceARN: aws.String(kf["resource_arn"].(string)),
		}
		output.KinesisFirehoseOutput = kfo
	}

	if v := o["kinesis_stream"].([]interface{}); len(v) > 0 {
		ks := v[0].(map[string]interface{})
		kso := &kinesisanalyticsv2.KinesisStreamsOutput{
			ResourceARN: aws.String(ks["resource_arn"].(string)),
		}
		output.KinesisStreamsOutput = kso
	}

	if v := o["lambda"].([]interface{}); len(v) > 0 {
		l := v[0].(map[string]interface{})
		lo := &kinesisanalyticsv2.LambdaOutput{
			ResourceARN: aws.String(l["resource_arn"].(string)),
		}
		output.LambdaOutput = lo
	}

	if v := o["schema"].([]interface{}); len(v) > 0 {
		ds := v[0].(map[string]interface{})
		dso := &kinesisanalyticsv2.DestinationSchema{
			RecordFormatType: aws.String(ds["record_format_type"].(string)),
		}
		output.DestinationSchema = dso
	}

	return output
}

func expandKinesisAnalyticsV2SourceSchema(vL map[string]interface{}) *kinesisanalyticsv2.SourceSchema {
	ss := &kinesisanalyticsv2.SourceSchema{}
	if v := vL["record_columns"].([]interface{}); len(v) > 0 {
		var rcs []*kinesisanalyticsv2.RecordColumn

		for _, rc := range v {
			rcD := rc.(map[string]interface{})
			rc := &kinesisanalyticsv2.RecordColumn{
				Name:    aws.String(rcD["name"].(string)),
				SqlType: aws.String(rcD["sql_type"].(string)),
			}

			if v, ok := rcD["mapping"]; ok {
				rc.Mapping = aws.String(v.(string))
			}

			rcs = append(rcs, rc)
		}

		ss.RecordColumns = rcs
	}

	if v, ok := vL["record_encoding"]; ok && v.(string) != "" {
		ss.RecordEncoding = aws.String(v.(string))
	}

	if v := vL["record_format"].([]interface{}); len(v) > 0 {
		vL := v[0].(map[string]interface{})
		rf := &kinesisanalyticsv2.RecordFormat{}

		if v := vL["mapping_parameters"].([]interface{}); len(v) > 0 {
			vL := v[0].(map[string]interface{})
			mp := &kinesisanalyticsv2.MappingParameters{}

			if v := vL["csv"].([]interface{}); len(v) > 0 {
				cL := v[0].(map[string]interface{})
				cmp := &kinesisanalyticsv2.CSVMappingParameters{
					RecordColumnDelimiter: aws.String(cL["record_column_delimiter"].(string)),
					RecordRowDelimiter:    aws.String(cL["record_row_delimiter"].(string)),
				}
				mp.CSVMappingParameters = cmp
				rf.RecordFormatType = aws.String("CSV")
			}

			if v := vL["json"].([]interface{}); len(v) > 0 {
				jL := v[0].(map[string]interface{})
				jmp := &kinesisanalyticsv2.JSONMappingParameters{
					RecordRowPath: aws.String(jL["record_row_path"].(string)),
				}
				mp.JSONMappingParameters = jmp
				rf.RecordFormatType = aws.String("JSON")
			}
			rf.MappingParameters = mp
		}

		ss.RecordFormat = rf
	}
	return ss
}

func expandKinesisAnalyticsV2ReferenceData(rd map[string]interface{}) *kinesisanalyticsv2.ReferenceDataSource {
	referenceData := &kinesisanalyticsv2.ReferenceDataSource{
		TableName: aws.String(rd["table_name"].(string)),
	}

	if v := rd["s3"].([]interface{}); len(v) > 0 {
		s3 := v[0].(map[string]interface{})
		s3rds := &kinesisanalyticsv2.S3ReferenceDataSource{
			BucketARN: aws.String(s3["bucket_arn"].(string)),
			FileKey:   aws.String(s3["file_key"].(string)),
		}
		referenceData.S3ReferenceDataSource = s3rds
	}

	if v := rd["schema"].([]interface{}); len(v) > 0 {
		ss := expandKinesisAnalyticsV2SourceSchema(v[0].(map[string]interface{}))
		referenceData.ReferenceSchema = ss
	}

	return referenceData
}

func createApplicationV2UpdateOpts(d *schema.ResourceData) (*kinesisanalyticsv2.UpdateApplicationInput, error) {
	applicationUpdate := &kinesisanalyticsv2.UpdateApplicationInput{}

	if d.HasChange("code") {
		if v, ok := d.GetOk("code"); ok && v.(string) != "" {
			applicationUpdate.ApplicationConfigurationUpdate = &kinesisanalyticsv2.ApplicationConfigurationUpdate{
				ApplicationCodeConfigurationUpdate: &kinesisanalyticsv2.ApplicationCodeConfigurationUpdate{
					CodeContentUpdate: &kinesisanalyticsv2.CodeContentUpdate{
						TextContentUpdate: aws.String(v.(string)),
					},
				},
			}
		}
	}

	oldLoggingOptions, newLoggingOptions := d.GetChange("cloudwatch_logging_options")
	if len(oldLoggingOptions.([]interface{})) > 0 && len(newLoggingOptions.([]interface{})) > 0 {
		if v, ok := d.GetOk("cloudwatch_logging_options"); ok {
			clo := v.([]interface{})[0].(map[string]interface{})
			cloudwatchLoggingOption := expandKinesisAnalyticsV2CloudwatchLoggingOptionUpdate(clo)
			applicationUpdate.CloudWatchLoggingOptionUpdates = []*kinesisanalyticsv2.CloudWatchLoggingOptionUpdate{cloudwatchLoggingOption}
		}
	}

	runtime := d.Get("runtime").(string)
	var sqlUpdate *kinesisanalyticsv2.SqlApplicationConfigurationUpdate
	if runtime == kinesisanalyticsv2.RuntimeEnvironmentSql10 {
		sqlUpdate = createKinesisAnalyticsV2SqlUpdateOpts(d)
	}

	var flinkUpdate *kinesisanalyticsv2.FlinkApplicationConfigurationUpdate
	if runtimeIsFlink(runtime) {
		flinkUpdate = createKinesisAnalyticsFlinkUpdateOpts(d)
	}

	var propertyGroupsUpdate *kinesisanalyticsv2.EnvironmentPropertyUpdates
	if d.HasChange("property_groups") {
		propertyGroupsUpdate = &kinesisanalyticsv2.EnvironmentPropertyUpdates{
			PropertyGroups: expandPropertyGroups(d.Get("property_groups").([]interface{})),
		}
	}

	var snapshotUpdate *kinesisanalyticsv2.ApplicationSnapshotConfigurationUpdate
	if d.HasChange("snapshots_enabled") {
		snapshotUpdate = &kinesisanalyticsv2.ApplicationSnapshotConfigurationUpdate{
			SnapshotsEnabledUpdate: aws.Bool(d.Get("snapshots_enabled").(bool)),
		}
	}

	if sqlUpdate != nil || flinkUpdate != nil || propertyGroupsUpdate != nil || snapshotUpdate != nil {
		applicationUpdate = &kinesisanalyticsv2.UpdateApplicationInput{
			ApplicationConfigurationUpdate: &kinesisanalyticsv2.ApplicationConfigurationUpdate{
				SqlApplicationConfigurationUpdate:      sqlUpdate,
				FlinkApplicationConfigurationUpdate:    flinkUpdate,
				EnvironmentPropertyUpdates:             propertyGroupsUpdate,
				ApplicationSnapshotConfigurationUpdate: snapshotUpdate,
			},
		}
	}

	return applicationUpdate, nil
}

func createKinesisAnalyticsV2SqlUpdateOpts(d *schema.ResourceData) *kinesisanalyticsv2.SqlApplicationConfigurationUpdate {
	var sqlUpdate *kinesisanalyticsv2.SqlApplicationConfigurationUpdate
	var inputsUpdate []*kinesisanalyticsv2.InputUpdate
	var outputsUpdate []*kinesisanalyticsv2.OutputUpdate
	var referenceDataUpdate []*kinesisanalyticsv2.ReferenceDataSourceUpdate

	sc := d.Get("sql_application_configuration").([]interface{})[0].(map[string]interface{})
	oldConfigIfc, _ := d.GetChange("sql_application_configuration")
	oldConfig := oldConfigIfc.([]interface{})
	var hasOldInputs, hasOldOutputs bool
	if len(oldConfig) > 0 {
		hasOldInputs = len(oldConfig[0].(map[string]interface{})["inputs"].([]interface{})) > 0
		hasOldOutputs = len(oldConfig[0].(map[string]interface{})["outputs"].([]interface{})) > 0
	}
	if hasOldInputs {
		if iConf, ok := sc["inputs"].([]interface{}); ok && len(iConf) > 0 {
			inputsUpdate = []*kinesisanalyticsv2.InputUpdate{expandKinesisAnalyticsV2InputUpdate(iConf[0].(map[string]interface{}))}
		}
	}
	if hasOldOutputs {
		if oConf, ok := sc["outputs"].([]interface{}); ok && len(oConf) > 0 {
			outputsUpdate = []*kinesisanalyticsv2.OutputUpdate{expandKinesisAnalyticsV2OutputUpdate(oConf[0].(map[string]interface{}))}
		}
	}
	rConf := sc["reference_data_sources"].([]interface{})
	for _, rd := range rConf {
		rdL := rd.(map[string]interface{})
		rdsu := &kinesisanalyticsv2.ReferenceDataSourceUpdate{
			ReferenceId:     aws.String(rdL["id"].(string)),
			TableNameUpdate: aws.String(rdL["table_name"].(string)),
		}

		if v := rdL["s3"].([]interface{}); len(v) > 0 {
			vL := v[0].(map[string]interface{})
			s3rdsu := &kinesisanalyticsv2.S3ReferenceDataSourceUpdate{
				BucketARNUpdate: aws.String(vL["bucket_arn"].(string)),
				FileKeyUpdate:   aws.String(vL["file_key"].(string)),
			}
			rdsu.S3ReferenceDataSourceUpdate = s3rdsu
		}

		if v := rdL["schema"].([]interface{}); len(v) > 0 {
			vL := v[0].(map[string]interface{})
			ss := expandKinesisAnalyticsV2SourceSchema(vL)
			rdsu.ReferenceSchemaUpdate = ss
		}

		referenceDataUpdate = append(referenceDataUpdate, rdsu)
	}
	if inputsUpdate != nil || outputsUpdate != nil || referenceDataUpdate != nil {
		sqlUpdate = &kinesisanalyticsv2.SqlApplicationConfigurationUpdate{
			InputUpdates:               inputsUpdate,
			OutputUpdates:              outputsUpdate,
			ReferenceDataSourceUpdates: referenceDataUpdate,
		}
	}
	return sqlUpdate
}

func createKinesisAnalyticsFlinkUpdateOpts(d *schema.ResourceData) *kinesisanalyticsv2.FlinkApplicationConfigurationUpdate {
	var flinkUpdate *kinesisanalyticsv2.FlinkApplicationConfigurationUpdate
	var checkpointUpdate *kinesisanalyticsv2.CheckpointConfigurationUpdate
	var monitoringUpdate *kinesisanalyticsv2.MonitoringConfigurationUpdate
	var parallelismUpdate *kinesisanalyticsv2.ParallelismConfigurationUpdate
	if d.HasChange("flink_application_configuration") {
		fc := d.Get("flink_application_configuration").([]interface{})[0].(map[string]interface{})
		cpConf := fc["checkpoint_configuration"].(*schema.Set)
		checkpointConfig := expandCheckpointConfiguration(cpConf)
		checkpointUpdate = &kinesisanalyticsv2.CheckpointConfigurationUpdate{
			CheckpointIntervalUpdate:         checkpointConfig.CheckpointInterval,
			CheckpointingEnabledUpdate:       checkpointConfig.CheckpointingEnabled,
			ConfigurationTypeUpdate:          checkpointConfig.ConfigurationType,
			MinPauseBetweenCheckpointsUpdate: checkpointConfig.MinPauseBetweenCheckpoints,
		}
		montConf := fc["monitoring_configuration"].(*schema.Set)
		monitoringConfig := expandMonitoringConfiguration(montConf)
		monitoringUpdate = &kinesisanalyticsv2.MonitoringConfigurationUpdate{
			ConfigurationTypeUpdate: monitoringConfig.ConfigurationType,
			LogLevelUpdate:          monitoringConfig.LogLevel,
			MetricsLevelUpdate:      monitoringConfig.MetricsLevel,
		}
		paraConf := fc["parallelism_configuration"].(*schema.Set)
		parallelismConfig := expandParallelismConfiguration(paraConf)
		parallelismUpdate = &kinesisanalyticsv2.ParallelismConfigurationUpdate{
			AutoScalingEnabledUpdate: parallelismConfig.AutoScalingEnabled,
			ConfigurationTypeUpdate:  parallelismConfig.ConfigurationType,
			ParallelismUpdate:        parallelismConfig.Parallelism,
			ParallelismPerKPUUpdate:  parallelismConfig.ParallelismPerKPU,
		}
	}

	if checkpointUpdate != nil || monitoringUpdate != nil || parallelismUpdate != nil {
		flinkUpdate = &kinesisanalyticsv2.FlinkApplicationConfigurationUpdate{
			CheckpointConfigurationUpdate:  checkpointUpdate,
			MonitoringConfigurationUpdate:  monitoringUpdate,
			ParallelismConfigurationUpdate: parallelismUpdate,
		}
	}
	return flinkUpdate
}

func expandCheckpointConfiguration(config *schema.Set) *kinesisanalyticsv2.CheckpointConfiguration {
	var checkpointingEnabled *bool
	var checkpointInterval *int64
	var configurationType *string
	var checkpointMinPause *int64

	for _, v := range config.List() {
		m := v.(map[string]interface{})
		if interval, ok := m["checkpoint_interval"]; ok {
			checkpointInterval = aws.Int64(int64(interval.(int)))
		}
		if enabled, ok := m["checkpointing_enabled"]; ok {
			checkpointingEnabled = aws.Bool(enabled.(bool))
		}
		if confType, ok := m["configuration_type"]; ok {
			configurationType = aws.String(confType.(string))
		}
		if minPause, ok := m["min_pause_between_checkpoints"]; ok {
			checkpointMinPause = aws.Int64(int64(minPause.(int)))
		}
	}
	return &kinesisanalyticsv2.CheckpointConfiguration{
		CheckpointInterval:         checkpointInterval,
		CheckpointingEnabled:       checkpointingEnabled,
		ConfigurationType:          configurationType,
		MinPauseBetweenCheckpoints: checkpointMinPause,
	}
}

func expandMonitoringConfiguration(config *schema.Set) *kinesisanalyticsv2.MonitoringConfiguration {
	var configurationType *string
	var logLevel *string
	var metricsLevel *string

	for _, v := range config.List() {
		m := v.(map[string]interface{})
		if confType, ok := m["configuration_type"]; ok {
			configurationType = aws.String(confType.(string))
		}
		if level, ok := m["log_level"]; ok {
			logLevel = aws.String(level.(string))
		}
		if level, ok := m["metrics_level"]; ok {
			metricsLevel = aws.String(level.(string))
		}
	}
	return &kinesisanalyticsv2.MonitoringConfiguration{
		ConfigurationType: configurationType,
		LogLevel:          logLevel,
		MetricsLevel:      metricsLevel,
	}
}

func expandParallelismConfiguration(config *schema.Set) *kinesisanalyticsv2.ParallelismConfiguration {
	var autoscalingEnabled *bool
	var configurationType *string
	var parallelism *int64
	var parallelismPerKPU *int64

	for _, v := range config.List() {
		m := v.(map[string]interface{})
		if aEnabled, ok := m["autoscaling_enabled"]; ok {
			autoscalingEnabled = aws.Bool(aEnabled.(bool))
		}
		if confType, ok := m["configuration_type"]; ok {
			configurationType = aws.String(confType.(string))
		}
		if p, ok := m["parallelism"]; ok {
			parallelism = aws.Int64(int64(p.(int)))
		}
		if p, ok := m["parallelism_per_kpu"]; ok {
			parallelismPerKPU = aws.Int64(int64(p.(int)))
		}
	}
	return &kinesisanalyticsv2.ParallelismConfiguration{
		AutoScalingEnabled: autoscalingEnabled,
		ConfigurationType:  configurationType,
		Parallelism:        parallelism,
		ParallelismPerKPU:  parallelismPerKPU,
	}
}

func expandKinesisAnalyticsV2InputUpdate(vL map[string]interface{}) *kinesisanalyticsv2.InputUpdate {
	inputUpdate := &kinesisanalyticsv2.InputUpdate{
		InputId:          aws.String(vL["id"].(string)),
		NamePrefixUpdate: aws.String(vL["name_prefix"].(string)),
	}

	if v := vL["kinesis_firehose"].([]interface{}); len(v) > 0 {
		kf := v[0].(map[string]interface{})
		kfiu := &kinesisanalyticsv2.KinesisFirehoseInputUpdate{
			ResourceARNUpdate: aws.String(kf["resource_arn"].(string)),
		}
		inputUpdate.KinesisFirehoseInputUpdate = kfiu
	}

	if v := vL["kinesis_stream"].([]interface{}); len(v) > 0 {
		ks := v[0].(map[string]interface{})
		ksiu := &kinesisanalyticsv2.KinesisStreamsInputUpdate{
			ResourceARNUpdate: aws.String(ks["resource_arn"].(string)),
		}
		inputUpdate.KinesisStreamsInputUpdate = ksiu
	}

	if v := vL["parallelism"].([]interface{}); len(v) > 0 {
		p := v[0].(map[string]interface{})

		if c, ok := p["count"]; ok {
			ipu := &kinesisanalyticsv2.InputParallelismUpdate{
				CountUpdate: aws.Int64(int64(c.(int))),
			}
			inputUpdate.InputParallelismUpdate = ipu
		}
	}

	if v := vL["processing_configuration"].([]interface{}); len(v) > 0 {
		pc := v[0].(map[string]interface{})

		if l := pc["lambda"].([]interface{}); len(l) > 0 {
			lp := l[0].(map[string]interface{})
			ipc := &kinesisanalyticsv2.InputProcessingConfigurationUpdate{
				InputLambdaProcessorUpdate: &kinesisanalyticsv2.InputLambdaProcessorUpdate{
					ResourceARNUpdate: aws.String(lp["resource_arn"].(string)),
				},
			}
			inputUpdate.InputProcessingConfigurationUpdate = ipc
		}
	}

	if v := vL["schema"].([]interface{}); len(v) > 0 {
		ss := &kinesisanalyticsv2.InputSchemaUpdate{}
		vL := v[0].(map[string]interface{})

		if v := vL["record_columns"].([]interface{}); len(v) > 0 {
			var rcs []*kinesisanalyticsv2.RecordColumn

			for _, rc := range v {
				rcD := rc.(map[string]interface{})
				rc := &kinesisanalyticsv2.RecordColumn{
					Name:    aws.String(rcD["name"].(string)),
					SqlType: aws.String(rcD["sql_type"].(string)),
				}

				if v, ok := rcD["mapping"]; ok {
					rc.Mapping = aws.String(v.(string))
				}

				rcs = append(rcs, rc)
			}

			ss.RecordColumnUpdates = rcs
		}

		if v, ok := vL["record_encoding"]; ok && v.(string) != "" {
			ss.RecordEncodingUpdate = aws.String(v.(string))
		}

		if v := vL["record_format"].([]interface{}); len(v) > 0 {
			vL := v[0].(map[string]interface{})
			rf := &kinesisanalyticsv2.RecordFormat{}

			if v := vL["mapping_parameters"].([]interface{}); len(v) > 0 {
				vL := v[0].(map[string]interface{})
				mp := &kinesisanalyticsv2.MappingParameters{}

				if v := vL["csv"].([]interface{}); len(v) > 0 {
					cL := v[0].(map[string]interface{})
					cmp := &kinesisanalyticsv2.CSVMappingParameters{
						RecordColumnDelimiter: aws.String(cL["record_column_delimiter"].(string)),
						RecordRowDelimiter:    aws.String(cL["record_row_delimiter"].(string)),
					}
					mp.CSVMappingParameters = cmp
					rf.RecordFormatType = aws.String("CSV")
				}

				if v := vL["json"].([]interface{}); len(v) > 0 {
					jL := v[0].(map[string]interface{})
					jmp := &kinesisanalyticsv2.JSONMappingParameters{
						RecordRowPath: aws.String(jL["record_row_path"].(string)),
					}
					mp.JSONMappingParameters = jmp
					rf.RecordFormatType = aws.String("JSON")
				}
				rf.MappingParameters = mp
			}
			ss.RecordFormatUpdate = rf
		}
		inputUpdate.InputSchemaUpdate = ss
	}

	return inputUpdate
}

func expandKinesisAnalyticsV2OutputUpdate(vL map[string]interface{}) *kinesisanalyticsv2.OutputUpdate {
	outputUpdate := &kinesisanalyticsv2.OutputUpdate{
		OutputId:   aws.String(vL["id"].(string)),
		NameUpdate: aws.String(vL["name"].(string)),
	}

	if v := vL["kinesis_firehose"].([]interface{}); len(v) > 0 {
		kf := v[0].(map[string]interface{})
		kfou := &kinesisanalyticsv2.KinesisFirehoseOutputUpdate{
			ResourceARNUpdate: aws.String(kf["resource_arn"].(string)),
		}
		outputUpdate.KinesisFirehoseOutputUpdate = kfou
	}

	if v := vL["kinesis_stream"].([]interface{}); len(v) > 0 {
		ks := v[0].(map[string]interface{})
		ksou := &kinesisanalyticsv2.KinesisStreamsOutputUpdate{
			ResourceARNUpdate: aws.String(ks["resource_arn"].(string)),
		}
		outputUpdate.KinesisStreamsOutputUpdate = ksou
	}

	if v := vL["lambda"].([]interface{}); len(v) > 0 {
		l := v[0].(map[string]interface{})
		lou := &kinesisanalyticsv2.LambdaOutputUpdate{
			ResourceARNUpdate: aws.String(l["resource_arn"].(string)),
		}
		outputUpdate.LambdaOutputUpdate = lou
	}

	if v := vL["schema"].([]interface{}); len(v) > 0 {
		ds := v[0].(map[string]interface{})
		dsu := &kinesisanalyticsv2.DestinationSchema{
			RecordFormatType: aws.String(ds["record_format_type"].(string)),
		}
		outputUpdate.DestinationSchemaUpdate = dsu
	}

	return outputUpdate
}

func expandKinesisAnalyticsV2CloudwatchLoggingOptionUpdate(clo map[string]interface{}) *kinesisanalyticsv2.CloudWatchLoggingOptionUpdate {
	cloudwatchLoggingOption := &kinesisanalyticsv2.CloudWatchLoggingOptionUpdate{
		CloudWatchLoggingOptionId: aws.String(clo["id"].(string)),
		LogStreamARNUpdate:        aws.String(clo["log_stream_arn"].(string)),
	}
	return cloudwatchLoggingOption
}

func expandKinesisAnalayticsSqlApplicationConfiguration(appConfig map[string]interface{}) *kinesisanalyticsv2.SqlApplicationConfiguration {
	sqlApplicationConfiguration := &kinesisanalyticsv2.SqlApplicationConfiguration{}
	if v := appConfig["inputs"].([]interface{}); len(v) > 0 {
		i := v[0].(map[string]interface{})
		inputs := expandKinesisAnalyticsV2Inputs(i)
		sqlApplicationConfiguration.Inputs = []*kinesisanalyticsv2.Input{inputs}
	}
	if v := appConfig["outputs"].([]interface{}); len(v) > 0 {
		outputs := make([]*kinesisanalyticsv2.Output, 0)
		for _, o := range v {
			output := expandKinesisAnalyticsV2Outputs(o.(map[string]interface{}))
			outputs = append(outputs, output)
		}
		sqlApplicationConfiguration.Outputs = outputs
	}
	if v := appConfig["reference_data_sources"].([]interface{}); len(v) > 0 {
		references := make([]*kinesisanalyticsv2.ReferenceDataSource, 0)
		for _, r := range v {
			references = append(references, expandKinesisAnalyticsV2ReferenceData(r.(map[string]interface{})))
		}
		sqlApplicationConfiguration.ReferenceDataSources = references
	}
	return sqlApplicationConfiguration
}

func expandKinesisAnalyticsFlinkApplicationConfiguration(appConfig map[string]interface{}) *kinesisanalyticsv2.FlinkApplicationConfiguration {
	flinkApplicationConfiguration := &kinesisanalyticsv2.FlinkApplicationConfiguration{}

	flinkApplicationConfiguration.CheckpointConfiguration = expandCheckpointConfiguration(appConfig["checkpoint_configuration"].(*schema.Set))
	flinkApplicationConfiguration.MonitoringConfiguration = expandMonitoringConfiguration(appConfig["monitoring_configuration"].(*schema.Set))
	flinkApplicationConfiguration.ParallelismConfiguration = expandParallelismConfiguration(appConfig["parallelism_configuration"].(*schema.Set))

	return flinkApplicationConfiguration
}

func flattenSqlApplicationConfigurationDescription(sqlApplicationConfig *kinesisanalyticsv2.SqlApplicationConfigurationDescription) []interface{} {
	ret := map[string]interface{}{}

	if sqlApplicationConfig == nil {
		return []interface{}{ret}
	}

	ret["inputs"] = flattenKinesisAnalyticsV2Inputs(sqlApplicationConfig.InputDescriptions)
	ret["outputs"] = flattenKinesisAnalyticsV2Outputs(sqlApplicationConfig.OutputDescriptions)
	ret["reference_data_sources"] = flattenKinesisAnalyticsV2ReferenceDataSources(sqlApplicationConfig.ReferenceDataSourceDescriptions)
	return []interface{}{ret}
}

func flattenFlinkApplicationConfigurationDescription(flinkApplicationConfig *kinesisanalyticsv2.FlinkApplicationConfigurationDescription) []interface{} {
	if flinkApplicationConfig == nil {
		return []interface{}{}
	}
	return []interface{}{map[string]interface{}{
		"checkpoint_configuration":  flattenCheckpointConfiguration(flinkApplicationConfig.CheckpointConfigurationDescription),
		"monitoring_configuration":  flattenMonitoringConfiguration(flinkApplicationConfig.MonitoringConfigurationDescription),
		"parallelism_configuration": flattenParallelismConfiguration(flinkApplicationConfig.ParallelismConfigurationDescription),
	},
	}
}

func flattenCheckpointConfiguration(checkpointConfiguration *kinesisanalyticsv2.CheckpointConfigurationDescription) *schema.Set {
	return schema.NewSet(resourceCheckpointConfigurationHash, []interface{}{map[string]interface{}{
		"checkpoint_interval":           aws.Int64Value(checkpointConfiguration.CheckpointInterval),
		"checkpointing_enabled":         aws.BoolValue(checkpointConfiguration.CheckpointingEnabled),
		"configuration_type":            aws.StringValue(checkpointConfiguration.ConfigurationType),
		"min_pause_between_checkpoints": aws.Int64Value(checkpointConfiguration.MinPauseBetweenCheckpoints),
	}})
}

func flattenMonitoringConfiguration(monitoringConfiguration *kinesisanalyticsv2.MonitoringConfigurationDescription) *schema.Set {
	return schema.NewSet(resourceMonitoringConfigurationHash, []interface{}{map[string]interface{}{
		"configuration_type": aws.StringValue(monitoringConfiguration.ConfigurationType),
		"log_level":          aws.StringValue(monitoringConfiguration.LogLevel),
		"metrics_level":      aws.StringValue(monitoringConfiguration.MetricsLevel),
	}})
}

func flattenParallelismConfiguration(parallelismConfiguration *kinesisanalyticsv2.ParallelismConfigurationDescription) *schema.Set {
	return schema.NewSet(resourceParallelismConfigurationHash, []interface{}{map[string]interface{}{
		"autoscaling_enabled": aws.BoolValue(parallelismConfiguration.AutoScalingEnabled),
		"configuration_type":  aws.StringValue(parallelismConfiguration.ConfigurationType),
		"parallelism":         aws.Int64Value(parallelismConfiguration.Parallelism),
		"parallelism_per_kpu": aws.Int64Value(parallelismConfiguration.ParallelismPerKPU),
	}})
}

func flattenKinesisAnalyticsPropertyGroups(propGroups []*kinesisanalyticsv2.PropertyGroup) []interface{} {
	s := []interface{}{}
	for _, v := range propGroups {
		propMap := make(map[string]interface{})
		for k, v := range v.PropertyMap {
			propMap[k] = aws.StringValue(v)
		}
		group := map[string]interface{}{
			"property_group_id": aws.StringValue(v.PropertyGroupId),
			"property_map":      propMap,
		}
		s = append(s, group)
	}
	return s
}

func flattenKinesisAnalyticsV2CloudwatchLoggingOptions(options []*kinesisanalyticsv2.CloudWatchLoggingOptionDescription) []interface{} {
	s := []interface{}{}
	for _, v := range options {
		option := map[string]interface{}{
			"id":             aws.StringValue(v.CloudWatchLoggingOptionId),
			"log_stream_arn": aws.StringValue(v.LogStreamARN),
		}
		s = append(s, option)
	}
	return s
}

func flattenKinesisAnalyticsV2Inputs(inputs []*kinesisanalyticsv2.InputDescription) []interface{} {
	s := []interface{}{}

	if len(inputs) == 0 {
		return s
	}
	id := inputs[0]

	input := map[string]interface{}{
		"id":          aws.StringValue(id.InputId),
		"name_prefix": aws.StringValue(id.NamePrefix),
	}

	list := schema.NewSet(schema.HashString, nil)
	for _, sn := range id.InAppStreamNames {
		list.Add(aws.StringValue(sn))
	}
	input["stream_names"] = list

	if id.InputParallelism != nil {
		input["parallelism"] = []interface{}{
			map[string]interface{}{
				"count": int(aws.Int64Value(id.InputParallelism.Count)),
			},
		}
	}

	if id.InputProcessingConfigurationDescription != nil {
		ipcd := id.InputProcessingConfigurationDescription

		if ipcd.InputLambdaProcessorDescription != nil {
			input["processing_configuration"] = []interface{}{
				map[string]interface{}{
					"lambda": []interface{}{
						map[string]interface{}{
							"resource_arn": aws.StringValue(ipcd.InputLambdaProcessorDescription.ResourceARN),
						},
					},
				},
			}
		}
	}

	if id.InputSchema != nil {
		inputSchema := id.InputSchema
		is := []interface{}{}
		rcs := []interface{}{}
		ss := map[string]interface{}{
			"record_encoding": aws.StringValue(inputSchema.RecordEncoding),
		}

		for _, rc := range inputSchema.RecordColumns {
			rcM := map[string]interface{}{
				"mapping":  aws.StringValue(rc.Mapping),
				"name":     aws.StringValue(rc.Name),
				"sql_type": aws.StringValue(rc.SqlType),
			}
			rcs = append(rcs, rcM)
		}
		ss["record_columns"] = rcs

		if inputSchema.RecordFormat != nil {
			rf := inputSchema.RecordFormat
			rfM := map[string]interface{}{
				"record_format_type": aws.StringValue(rf.RecordFormatType),
			}

			if rf.MappingParameters != nil {
				mps := []interface{}{}
				if rf.MappingParameters.CSVMappingParameters != nil {
					cmp := map[string]interface{}{
						"csv": []interface{}{
							map[string]interface{}{
								"record_column_delimiter": aws.StringValue(rf.MappingParameters.CSVMappingParameters.RecordColumnDelimiter),
								"record_row_delimiter":    aws.StringValue(rf.MappingParameters.CSVMappingParameters.RecordRowDelimiter),
							},
						},
					}
					mps = append(mps, cmp)
				}

				if rf.MappingParameters.JSONMappingParameters != nil {
					jmp := map[string]interface{}{
						"json": []interface{}{
							map[string]interface{}{
								"record_row_path": aws.StringValue(rf.MappingParameters.JSONMappingParameters.RecordRowPath),
							},
						},
					}
					mps = append(mps, jmp)
				}

				rfM["mapping_parameters"] = mps
			}
			ss["record_format"] = []interface{}{rfM}
		}

		is = append(is, ss)
		input["schema"] = is
	}

	if id.InputStartingPositionConfiguration != nil && id.InputStartingPositionConfiguration.InputStartingPosition != nil {
		input["starting_position_configuration"] = []interface{}{
			map[string]interface{}{
				"starting_position": aws.StringValue(id.InputStartingPositionConfiguration.InputStartingPosition),
			},
		}
	}

	if id.KinesisFirehoseInputDescription != nil {
		input["kinesis_firehose"] = []interface{}{
			map[string]interface{}{
				"resource_arn": aws.StringValue(id.KinesisFirehoseInputDescription.ResourceARN),
			},
		}
	}

	if id.KinesisStreamsInputDescription != nil {
		input["kinesis_stream"] = []interface{}{
			map[string]interface{}{
				"resource_arn": aws.StringValue(id.KinesisStreamsInputDescription.ResourceARN),
			},
		}
	}
	s = append(s, input)
	return s
}

func flattenKinesisAnalyticsV2Outputs(outputs []*kinesisanalyticsv2.OutputDescription) []interface{} {
	s := []interface{}{}

	for _, o := range outputs {
		output := map[string]interface{}{
			"id":   aws.StringValue(o.OutputId),
			"name": aws.StringValue(o.Name),
		}

		if o.KinesisFirehoseOutputDescription != nil {
			output["kinesis_firehose"] = []interface{}{
				map[string]interface{}{
					"resource_arn": aws.StringValue(o.KinesisFirehoseOutputDescription.ResourceARN),
				},
			}
		}

		if o.KinesisStreamsOutputDescription != nil {
			output["kinesis_stream"] = []interface{}{
				map[string]interface{}{
					"resource_arn": aws.StringValue(o.KinesisStreamsOutputDescription.ResourceARN),
				},
			}
		}

		if o.LambdaOutputDescription != nil {
			output["lambda"] = []interface{}{
				map[string]interface{}{
					"resource_arn": aws.StringValue(o.LambdaOutputDescription.ResourceARN),
				},
			}
		}

		if o.DestinationSchema != nil {
			output["schema"] = []interface{}{
				map[string]interface{}{
					"record_format_type": aws.StringValue(o.DestinationSchema.RecordFormatType),
				},
			}
		}

		s = append(s, output)
	}

	return s
}

func flattenKinesisAnalyticsV2ReferenceDataSources(dataSources []*kinesisanalyticsv2.ReferenceDataSourceDescription) []interface{} {
	s := []interface{}{}

	if len(dataSources) > 0 {
		for _, ds := range dataSources {
			dataSource := map[string]interface{}{
				"id":         aws.StringValue(ds.ReferenceId),
				"table_name": aws.StringValue(ds.TableName),
			}

			if ds.S3ReferenceDataSourceDescription != nil {
				dataSource["s3"] = []interface{}{
					map[string]interface{}{
						"bucket_arn": aws.StringValue(ds.S3ReferenceDataSourceDescription.BucketARN),
						"file_key":   aws.StringValue(ds.S3ReferenceDataSourceDescription.FileKey),
					},
				}
			}

			if ds.ReferenceSchema != nil {
				rs := ds.ReferenceSchema
				rcs := []interface{}{}
				ss := map[string]interface{}{
					"record_encoding": aws.StringValue(rs.RecordEncoding),
				}

				for _, rc := range rs.RecordColumns {
					rcM := map[string]interface{}{
						"mapping":  aws.StringValue(rc.Mapping),
						"name":     aws.StringValue(rc.Name),
						"sql_type": aws.StringValue(rc.SqlType),
					}
					rcs = append(rcs, rcM)
				}
				ss["record_columns"] = rcs

				if rs.RecordFormat != nil {
					rf := rs.RecordFormat
					rfM := map[string]interface{}{
						"record_format_type": aws.StringValue(rf.RecordFormatType),
					}

					if rf.MappingParameters != nil {
						mps := []interface{}{}
						if rf.MappingParameters.CSVMappingParameters != nil {
							cmp := map[string]interface{}{
								"csv": []interface{}{
									map[string]interface{}{
										"record_column_delimiter": aws.StringValue(rf.MappingParameters.CSVMappingParameters.RecordColumnDelimiter),
										"record_row_delimiter":    aws.StringValue(rf.MappingParameters.CSVMappingParameters.RecordRowDelimiter),
									},
								},
							}
							mps = append(mps, cmp)
						}

						if rf.MappingParameters.JSONMappingParameters != nil {
							jmp := map[string]interface{}{
								"json": []interface{}{
									map[string]interface{}{
										"record_row_path": aws.StringValue(rf.MappingParameters.JSONMappingParameters.RecordRowPath),
									},
								},
							}
							mps = append(mps, jmp)
						}

						rfM["mapping_parameters"] = mps
					}
					ss["record_format"] = []interface{}{rfM}
				}

				dataSource["schema"] = []interface{}{ss}
			}

			s = append(s, dataSource)
		}
	}

	return s
}

func waitForDeleteKinesisAnalyticsV2Application(conn *kinesisanalyticsv2.KinesisAnalyticsV2, applicationId string, timeout time.Duration) error {
	stateConf := resource.StateChangeConf{
		Pending: []string{
			kinesisanalyticsv2.ApplicationStatusRunning,
			kinesisanalyticsv2.ApplicationStatusDeleting,
		},
		Target:  []string{""},
		Timeout: timeout,
		Refresh: refreshKinesisAnalyticsApplicationStatusV2(conn, applicationId),
	}
	application, err := stateConf.WaitForState()
	if err != nil {
		if isAWSErr(err, kinesisanalyticsv2.ErrCodeResourceNotFoundException, "") {
			return nil
		}
	}
	if application == nil {
		return nil
	}
	return err
}

func refreshKinesisAnalyticsApplicationStatusV2(conn *kinesisanalyticsv2.KinesisAnalyticsV2, applicationId string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		output, err := conn.DescribeApplication(&kinesisanalyticsv2.DescribeApplicationInput{
			ApplicationName: aws.String(applicationId),
		})
		if err != nil {
			return nil, "", err
		}
		application := output.ApplicationDetail
		if application == nil {
			return application, "", fmt.Errorf("Kinesis Analytics Application (%s) could not be found.", applicationId)
		}
		return application, aws.StringValue(application.ApplicationStatus), nil
	}
}

func runtimeIsFlink(runtime string) bool {
	return runtime == kinesisanalyticsv2.RuntimeEnvironmentFlink16 ||
		runtime == kinesisanalyticsv2.RuntimeEnvironmentFlink18
}

func resourceCheckpointConfigurationHash(v interface{}) int {
	var buf bytes.Buffer
	m := v.(map[string]interface{})

	if v, ok := m["checkpoint_interval"]; ok {
		buf.WriteString(fmt.Sprintf("%d-", v.(int64)))
	}
	if v, ok := m["checkpointing_enabled"]; ok {
		buf.WriteString(fmt.Sprintf("%t-", v.(bool)))
	}
	if v, ok := m["configuration_type"]; ok {
		buf.WriteString(fmt.Sprintf("%s-", v.(string)))
	}
	if v, ok := m["min_pause_between_checkpoints"]; ok {
		buf.WriteString(fmt.Sprintf("%d-", v.(int64)))
	}

	return hashcode.String(buf.String())
}

func resourceMonitoringConfigurationHash(v interface{}) int {
	var buf bytes.Buffer
	m := v.(map[string]interface{})

	if v, ok := m["log_level"]; ok {
		buf.WriteString(fmt.Sprintf("%s-", v.(string)))
	}
	if v, ok := m["configuration_type"]; ok {
		buf.WriteString(fmt.Sprintf("%s-", v.(string)))
	}
	if v, ok := m["metrics_level"]; ok {
		buf.WriteString(fmt.Sprintf("%s-", v.(string)))
	}

	return hashcode.String(buf.String())
}

func resourceParallelismConfigurationHash(v interface{}) int {
	var buf bytes.Buffer
	m := v.(map[string]interface{})

	if v, ok := m["autoscaling_enabled"]; ok {
		buf.WriteString(fmt.Sprintf("%t-", v.(bool)))
	}
	if v, ok := m["parallelism"]; ok {
		buf.WriteString(fmt.Sprintf("%d-", v.(int64)))
	}
	if v, ok := m["parallelism_per_kpu"]; ok {
		buf.WriteString(fmt.Sprintf("%d-", v.(int64)))
	}
	if v, ok := m["configuration_type"]; ok {
		buf.WriteString(fmt.Sprintf("%s-", v.(string)))
	}

	return hashcode.String(buf.String())
}